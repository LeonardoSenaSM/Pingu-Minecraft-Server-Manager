package manager

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"mineserver/internal/i18n"
)

const maxPreviewSize = 1024 * 1024

type vanillaBan struct {
	Name    string `json:"name"`
	IP      string `json:"ip"`
	Created string `json:"created"`
	Expires string `json:"expires"`
	Reason  string `json:"reason"`
}

func (m *Manager) settingsPath() string { return filepath.Join(m.serverDir, "config.json") }
func (m *Manager) paperInstallationPath() string {
	return filepath.Join(m.serverDir, "mineserver-paper-version.json")
}
func (m *Manager) managedBansPath() string {
	return filepath.Join(m.serverDir, "mineserver-bans.json")
}

func (m *Manager) ensureDirectories() error {
	for _, directory := range []string{m.serverDir, m.pluginsDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("criar pasta %s: %w", directory, err)
		}
	}
	eula := filepath.Join(m.serverDir, "eula.txt")
	if _, err := os.Stat(eula); errors.Is(err, os.ErrNotExist) {
		if err := writeBytesAtomic(eula, []byte("# Gerado pelo MineServer\neula=true\n")); err != nil {
			return fmt.Errorf("criar eula.txt: %w", err)
		}
	}
	return m.updateServerProperties(m.paperPort())
}

func (m *Manager) loadSettings() {
	data, err := os.ReadFile(m.settingsPath())
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			m.Log("warning", "storage", "não foi possível ler config.json", err)
		}
		return
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		m.Log("warning", "storage", "config.json inválido; usando padrões", err)
		return
	}
	m.mu.Lock()
	if strings.TrimSpace(settings.SelectedMinecraftVersion) != "" {
		m.settings.SelectedMinecraftVersion = settings.SelectedMinecraftVersion
	}
	if settings.Language == i18n.English || settings.Language == i18n.Portuguese {
		m.settings.Language = settings.Language
	}
	if strings.TrimSpace(settings.ServerName) != "" {
		m.settings.ServerName = strings.TrimSpace(settings.ServerName)
	}
	if settings.MaxPlayers >= 1 && settings.MaxPlayers <= 1000 {
		m.settings.MaxPlayers = settings.MaxPlayers
	}
	language := m.settings.Language
	m.mu.Unlock()
	m.notifyLanguage(language)
}

func (m *Manager) InstalledPaperVersion() string {
	data, err := os.ReadFile(m.paperInstallationPath())
	if err != nil {
		return "não identificada"
	}
	var installation PaperInstallation
	if err := json.Unmarshal(data, &installation); err != nil || installation.Version == "" {
		return "não identificada"
	}
	return installation.Version
}

func (m *Manager) updateServerProperties(paperPort int) error {
	if paperPort <= 0 {
		paperPort = javaPortStart + 1
	}
	m.mu.RLock()
	serverName := strings.TrimSpace(m.settings.ServerName)
	maxPlayers := m.settings.MaxPlayers
	m.mu.RUnlock()
	if serverName == "" {
		serverName = "MineServer"
	}
	if maxPlayers < 1 || maxPlayers > 1000 {
		maxPlayers = 20
	}
	path := filepath.Join(m.serverDir, "server.properties")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("ler server.properties: %w", err)
	}
	if errors.Is(err, os.ErrNotExist) {
		data = []byte("# Gerado pelo MineServer\nonline-mode=true\nenable-query=true\nview-distance=10\nsimulation-distance=8\n")
	}
	updates := map[string]string{
		"server-ip": "127.0.0.1", "server-port": strconv.Itoa(paperPort),
		"query.port": strconv.Itoa(paperPort), "max-players": strconv.Itoa(maxPlayers),
		"motd": serverName + " - Java + Bedrock",
	}
	seen := make(map[string]bool)
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			continue
		}
		key, _, found := strings.Cut(trimmed, "=")
		key = strings.TrimSpace(key)
		value, managed := updates[key]
		if found && managed {
			lines[index] = key + "=" + value
			seen[key] = true
		}
	}
	for _, key := range []string{"server-ip", "server-port", "query.port", "max-players", "motd"} {
		if !seen[key] {
			lines = append(lines, key+"="+updates[key])
		}
	}
	return writeBytesAtomic(path, []byte(strings.TrimRight(strings.Join(lines, "\n"), "\n")+"\n"))
}

func (m *Manager) configureGeyser(geyserPort, paperPort int) error {
	configPath := filepath.Join(m.pluginsDir, "Geyser-Spigot", "config.yml")
	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		data, err = geyserDefaultConfig(filepath.Join(m.pluginsDir, "Geyser-Spigot.jar"))
	}
	if err != nil {
		return fmt.Errorf("obter configuração do Geyser: %w", err)
	}
	m.mu.RLock()
	serverName := m.settings.ServerName
	m.mu.RUnlock()
	patched, err := patchGeyserConfig(string(data), geyserPort, paperPort, serverName)
	if err != nil {
		return err
	}
	return writeBytesAtomic(configPath, []byte(patched))
}

func geyserDefaultConfig(jarPath string) ([]byte, error) {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	for _, file := range reader.File {
		name := strings.ToLower(filepath.ToSlash(file.Name))
		if name != "config.yml" && !strings.HasSuffix(name, "/config.yml") {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(stream, 4*1024*1024))
		closeErr := stream.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(data) > 0 {
			return data, nil
		}
	}
	return []byte(`bedrock:
  address: 127.0.0.1
  port: 19133
  clone-remote-port: false
  motd1: "MineServer"
  motd2: "Java + Bedrock"
  server-name: "MineServer"
remote:
  address: 127.0.0.1
  port: 25566
  auth-type: floodgate
floodgate-key-file: key.pem
command-suggestions: true
passthrough-motd: false
passthrough-player-counts: true
use-direct-connection: true
disable-compression: true
config-version: 4
`), nil
}

func patchGeyserConfig(content string, geyserPort, paperPort int, serverName string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	section := ""
	replaced := make(map[string]bool)
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if strings.HasPrefix(trimmed, "#") {
			uncommented := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			if section == "bedrock" && strings.HasPrefix(uncommented, "address:") {
				lines[index] = line[:indent] + "address: 127.0.0.1"
				replaced["bedrock.address"] = true
			}
			continue
		}
		if indent == 0 && strings.HasSuffix(trimmed, ":") {
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if indent == 0 {
			section = ""
			continue
		}
		key, _, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value := ""
		switch section + "." + key {
		case "bedrock.address":
			value = "127.0.0.1"
		case "bedrock.port":
			value = strconv.Itoa(geyserPort)
		case "bedrock.server-name":
			value = strconv.Quote(serverName)
		case "remote.address":
			value = "127.0.0.1"
		case "remote.port":
			value = strconv.Itoa(paperPort)
		case "remote.auth-type":
			value = "floodgate"
		default:
			continue
		}
		lines[index] = line[:indent] + key + ": " + value
		replaced[section+"."+key] = true
	}
	required := []string{"bedrock.address", "bedrock.port", "bedrock.server-name", "remote.address", "remote.port", "remote.auth-type"}
	missing := make([]string, 0)
	for _, key := range required {
		if !replaced[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("configuração Geyser incompatível; campos ausentes: %s", strings.Join(missing, ", "))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n", nil
}

func (m *Manager) ListServerFiles() ([]FileEntry, error) {
	if err := os.MkdirAll(m.pluginsDir, 0o755); err != nil {
		return nil, err
	}
	result := make([]FileEntry, 0)
	err := filepath.WalkDir(m.serverDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == m.serverDir {
			return nil
		}
		relative, err := filepath.Rel(m.serverDir, path)
		if err != nil {
			return err
		}
		depth := strings.Count(filepath.ToSlash(relative), "/")
		icon := "📄 "
		if entry.IsDir() {
			icon = "📁 "
		}
		result = append(result, FileEntry{Relative: relative, Display: strings.Repeat("    ", depth) + icon + entry.Name(), IsDir: entry.IsDir()})
		return nil
	})
	return result, err
}

func (m *Manager) ReadServerFile(relative string) (string, error) {
	clean := filepath.Clean(relative)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("caminho inválido")
	}
	path := filepath.Join(m.serverDir, clean)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("o item selecionado é uma pasta")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolvedRelative, err := filepath.Rel(m.serverDir, resolved)
	if err != nil || resolvedRelative == ".." || strings.HasPrefix(resolvedRelative, ".."+string(os.PathSeparator)) {
		return "", errors.New("o caminho aponta para fora da pasta server")
	}
	if info.Size() > maxPreviewSize {
		return "", fmt.Errorf("arquivo maior que %d MB", maxPreviewSize/(1024*1024))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, value := range data[:min(len(data), 8000)] {
		if value == 0 {
			return "", errors.New("arquivo binário; visualização de texto bloqueada")
		}
	}
	return string(data), nil
}

func (m *Manager) BannedPlayers() []BanEntry {
	seen := make(map[string]bool)
	result := make([]BanEntry, 0)
	m.mu.RLock()
	for _, ban := range m.managedBans {
		result = append(result, ban)
		seen[strings.ToLower(ban.Name)+"|"+ban.IP] = true
	}
	m.mu.RUnlock()
	for _, name := range []string{"banned-players.json", "banned-ips.json"} {
		data, err := os.ReadFile(filepath.Join(m.serverDir, name))
		if err != nil {
			continue
		}
		var raw []vanillaBan
		if err := json.Unmarshal(data, &raw); err != nil {
			m.Log("warning", "storage", "arquivo de banimentos inválido: "+name, err)
			continue
		}
		for _, item := range raw {
			key := strings.ToLower(item.Name) + "|" + item.IP
			if seen[key] {
				continue
			}
			created, _ := time.Parse("2006-01-02 15:04:05 -0700", item.Created)
			expires, _ := time.Parse("2006-01-02 15:04:05 -0700", item.Expires)
			result = append(result, BanEntry{Name: item.Name, IP: item.IP, CreatedAt: created, ExpiresAt: expires, Reason: item.Reason})
			seen[key] = true
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

func (m *Manager) loadManagedBans() {
	data, err := os.ReadFile(m.managedBansPath())
	if err != nil {
		return
	}
	var bans []BanEntry
	if err := json.Unmarshal(data, &bans); err != nil {
		m.Log("warning", "storage", "mineserver-bans.json inválido", err)
		return
	}
	m.mu.Lock()
	m.managedBans = bans
	m.mu.Unlock()
}

func (m *Manager) loadKnownPlayers() {
	data, err := os.ReadFile(filepath.Join(m.serverDir, "usercache.json"))
	if err != nil {
		return
	}
	var cached []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &cached); err != nil {
		m.Log("warning", "storage", "usercache.json inválido", err)
		return
	}
	m.mu.Lock()
	for _, player := range cached {
		if name := strings.TrimSpace(player.Name); name != "" {
			m.knownPlayers[name] = time.Now()
		}
	}
	m.mu.Unlock()
}

func (m *Manager) saveManagedBans() error {
	m.mu.RLock()
	bans := append([]BanEntry(nil), m.managedBans...)
	m.mu.RUnlock()
	return writeJSONAtomic(m.managedBansPath(), bans)
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytesAtomic(path, data)
}

func writeBytesAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mineserver-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
		if err := os.Rename(tmpName, path); err != nil {
			return err
		}
	}
	committed = true
	return nil
}
