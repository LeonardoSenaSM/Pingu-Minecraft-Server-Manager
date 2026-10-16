package manager

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (m *Manager) backupsDir() string { return filepath.Join(m.serverDir, "backups") }

// levelName lê level-name do server.properties (padrão "world").
func (m *Manager) levelName() string {
	data, err := os.ReadFile(filepath.Join(m.serverDir, "server.properties"))
	if err != nil {
		return "world"
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && strings.TrimSpace(key) == "level-name" {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return "world"
}

// worldDirectories devolve as pastas do mundo: level-name e as variantes
// level-name_nether, level-name_the_end etc.
func (m *Manager) worldDirectories() ([]string, error) {
	level := m.levelName()
	entries, err := os.ReadDir(m.serverDir)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && (name == level || strings.HasPrefix(name, level+"_")) {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result, nil
}

// backupWorld compacta as pastas do mundo em server/backups. Retorna "" se
// ainda não existe mundo. O ZIP só aparece com o nome final quando completo.
func (m *Manager) backupWorld(ctx context.Context, fromVersion, toVersion string) (string, error) {
	folders, err := m.worldDirectories()
	if err != nil {
		return "", err
	}
	if len(folders) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(m.backupsDir(), 0o755); err != nil {
		return "", err
	}
	name := fmt.Sprintf("world-%s-para-%s-%s.zip",
		unsafeFileChars.ReplaceAllString(fromVersion, "_"),
		unsafeFileChars.ReplaceAllString(toVersion, "_"),
		time.Now().Format("20060102-150405"))
	destination := filepath.Join(m.backupsDir(), name)
	tmp, err := os.CreateTemp(m.backupsDir(), ".backup-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	archive := zip.NewWriter(tmp)
	for _, folder := range folders {
		root := filepath.Join(m.serverDir, folder)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := context.Cause(ctx); err != nil {
				return err
			}
			if entry.IsDir() || entry.Name() == "session.lock" {
				return nil
			}
			relative, err := filepath.Rel(m.serverDir, path)
			if err != nil {
				return err
			}
			return addFileToZip(archive, path, filepath.ToSlash(relative))
		})
		if err != nil {
			_ = archive.Close()
			return "", err
		}
	}
	if err := archive.Close(); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return "", err
	}
	committed = true
	return destination, nil
}

func addFileToZip(archive *zip.Writer, path, name string) error {
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = name
	header.Method = zip.Deflate
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, source)
	return err
}

// pruneBackups mantém só as `keep` entradas mais recentes com o prefixo dado.
// Os nomes têm carimbo de data, então a ordem alfabética é a cronológica.
func pruneBackups(dir, prefix string, keep int) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	var errs []error
	for len(names) > keep {
		if err := os.RemoveAll(filepath.Join(dir, names[0])); err != nil {
			errs = append(errs, err)
		}
		names = names[1:]
	}
	return errors.Join(errs...)
}
