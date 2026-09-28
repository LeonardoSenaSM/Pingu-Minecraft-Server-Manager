package manager

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLegacySettingsMigrateToAutomaticMemory(t *testing.T) {
	base := t.TempDir()
	serverDir := filepath.Join(base, "server")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{
		"selected_minecraft_version": "latest",
		"language":                   "en",
		"server_name":                "Legacy",
		"max_players":                12,
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(base)
	m.loadSettings()
	settings := m.Settings()
	if settings.MemoryMode != MemoryModeAutomatic || settings.MemoryLimitGB != 0 {
		t.Fatalf("legacy memory settings not migrated: %+v", settings)
	}
}

func TestSendCommandRejectsControlCharactersBeforeServerCheck(t *testing.T) {
	m := New(t.TempDir())
	if err := m.SendCommand("say hello\nstop"); err == nil || !strings.Contains(err.Error(), "caracteres inválidos") {
		t.Fatalf("unexpected validation result: %v", err)
	}
}

func TestSendCommandWhenServerIsStopped(t *testing.T) {
	m := New(t.TempDir())
	if err := m.SendCommand("list"); err == nil || !strings.Contains(err.Error(), "não está em execução") {
		t.Fatalf("unexpected stopped-server result: %v", err)
	}
}

func TestSendCommandWritesToServerStdin(t *testing.T) {
	m := New(t.TempDir())
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	m.serverStdin = writer
	if err := m.SendCommand("/say hello"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "say hello\n"; got != want {
		t.Fatalf("stdin = %q, want %q", got, want)
	}
}

func TestSettingsUpdateServerProperties(t *testing.T) {
	m := New(t.TempDir())
	t.Cleanup(func() { _ = m.Close() })
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveSettings("pt-BR", "Servidor Casa", 37); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(m.ServerDir(), "server.properties"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{"server-ip=127.0.0.1", "max-players=37", "motd=Servidor Casa - Java + Bedrock"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("server.properties não contém %q:\n%s", expected, text)
		}
	}
}

func TestManualMemorySettingsPersist(t *testing.T) {
	base := t.TempDir()
	m := New(base)
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	manualLimit := 1
	if m.totalMemoryGB >= 6 {
		manualLimit = 4
	}
	if err := m.SaveSettingsWithMemory("en", "Memory Test", 10, MemoryModeManual, manualLimit); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded := New(base)
	reloaded.loadSettings()
	settings := reloaded.Settings()
	if settings.MemoryMode != MemoryModeManual || settings.MemoryLimitGB != manualLimit {
		t.Fatalf("reloaded settings = %+v", settings)
	}
}

func TestConcurrentLogRingIsBounded(t *testing.T) {
	m := New(t.TempDir())
	var wait sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := 0; index < 200; index++ {
				m.Log("info", "test", "linha", nil)
			}
		}()
	}
	wait.Wait()
	m.mu.RLock()
	length := len(m.logs)
	m.mu.RUnlock()
	if length != maxLogEntries {
		t.Fatalf("buffer contém %d entradas; esperado %d", length, maxLogEntries)
	}
}

func TestPatchGeyserConfig(t *testing.T) {
	input := "bedrock:\n  #address: 0.0.0.0\n  port: 19132\n  server-name: Geyser\nremote:\n  address: auto\n  port: 25565\n  auth-type: online\nconfig-version: 4\n"
	patched, err := patchGeyserConfig(input, 19140, 25570, "Minha Rede")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"address: 127.0.0.1", "port: 19140", "port: 25570", "auth-type: floodgate", `server-name: "Minha Rede"`} {
		if !strings.Contains(patched, expected) {
			t.Fatalf("configuração não contém %q:\n%s", expected, patched)
		}
	}
}
