package manager

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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
