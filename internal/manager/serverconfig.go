package manager

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// prepareServerConfig ajusta arquivos do Paper antes de cada início.
//
// connection-throttle (bukkit.yml) limita reconexões vindas do mesmo IP. Como
// o proxy do Pingu entrega todas as conexões a partir de 127.0.0.1, o Paper
// enxerga todos os jogadores como um único IP e recusa quem entra logo depois
// de outro ("Connection throttled"). O limite não protege nada nesse arranjo,
// então é desativado (0).
func (m *Manager) prepareServerConfig() error {
	path := filepath.Join(m.serverDir, "bukkit.yml")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// O Bukkit completa os valores ausentes com os padrões e regrava o
		// arquivo no primeiro início.
		return writeBytesAtomic(path, []byte("settings:\n  connection-throttle: 0\n"))
	}
	if err != nil {
		return err
	}
	patched, changed := setNestedYAMLValue(string(data), "settings", "connection-throttle", "0")
	if !changed {
		return nil
	}
	if err := writeBytesAtomic(path, []byte(patched)); err != nil {
		return err
	}
	m.Log("info", "config", "connection-throttle desativado: o proxy do Pingu entrega todos os jogadores pelo mesmo endereço", nil)
	return nil
}

// setNestedYAMLValue altera section.key num YAML simples de dois níveis,
// preservando o resto do arquivo. Se a chave não existir, ela é adicionada à
// seção (ou a seção é criada). changed é false quando o valor já estava certo.
func setNestedYAMLValue(content, section, key, value string) (string, bool) {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	sectionIndex := -1
	insertAt := -1
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent == 0 {
			if sectionIndex >= 0 && insertAt < 0 {
				insertAt = index
			}
			if trimmed == section+":" {
				sectionIndex = index
			}
			continue
		}
		if sectionIndex < 0 || insertAt >= 0 {
			continue
		}
		currentKey, currentValue, found := strings.Cut(trimmed, ":")
		if !found || strings.TrimSpace(currentKey) != key {
			continue
		}
		if strings.TrimSpace(currentValue) == value {
			return content, false
		}
		lines[index] = line[:indent] + key + ": " + value
		return strings.Join(lines, newline), true
	}
	entry := "  " + key + ": " + value
	switch {
	case sectionIndex < 0:
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, section+":", entry, "")
	case insertAt < 0:
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, entry, "")
	default:
		lines = append(lines[:insertAt], append([]string{entry}, lines[insertAt:]...)...)
	}
	return strings.Join(lines, newline), true
}
