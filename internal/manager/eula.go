package manager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EULAURL é o endereço oficial da EULA do Minecraft.
const EULAURL = "https://aka.ms/MinecraftEULA"

// ErrEULANotAccepted indica que o dono do servidor ainda não aceitou a EULA.
var ErrEULANotAccepted = errors.New("é necessário aceitar a EULA do Minecraft antes de iniciar o servidor")

func (m *Manager) eulaPath() string { return filepath.Join(m.serverDir, "eula.txt") }

// EULAAccepted lê eula.txt. Instalações antigas do Pingu já gravaram
// eula=true e continuam funcionando sem nova pergunta.
func (m *Manager) EULAAccepted() bool {
	data, err := os.ReadFile(m.eulaPath())
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && strings.TrimSpace(key) == "eula" {
			return strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}
	return false
}

// AcceptEULA registra o aceite feito explicitamente pelo usuário na interface.
func (m *Manager) AcceptEULA() error {
	if err := os.MkdirAll(m.serverDir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("#By changing the setting below to TRUE you are indicating your agreement to our EULA (%s).\n# Aceita pelo usuário no Pingu em %s\neula=true\n", EULAURL, time.Now().Format(time.RFC3339))
	if err := writeBytesAtomic(m.eulaPath(), []byte(content)); err != nil {
		return fmt.Errorf("gravar eula.txt: %w", err)
	}
	m.Log("info", "eula", "EULA do Minecraft aceita pelo usuário", nil)
	m.notifyFiles()
	return nil
}
