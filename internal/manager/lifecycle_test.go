//go:build !windows

package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeJava cria runtime/bin/java: responde a -version como Java 25 e imita o
// console do Paper. Com ignoreStop, ignora "stop" (simula JVM travada).
func fakeJava(t *testing.T, base string, ignoreStop bool) {
	t.Helper()
	bin := filepath.Join(base, "runtime", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	stopAction := "echo '[Server thread/INFO]: Stopping server'; exit 0"
	if ignoreStop {
		stopAction = "echo ignorando"
	}
	script := `#!/bin/sh
if [ "$1" = "-version" ]; then echo 'openjdk version "25.0.1" 2025-10-21' >&2; exit 0; fi
echo '[Server thread/INFO]: Done (1.234s)! For help, type "help"'
while read line; do
  case "$line" in
    stop) ` + stopAction + ` ;;
    *) echo "[Server thread/INFO]: recebido $line" ;;
  esac
done
`
	if err := os.WriteFile(filepath.Join(bin, "java"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func newTestServer(t *testing.T, ignoreStop bool) *Manager {
	t.Helper()
	base := t.TempDir()
	fakeJava(t, base, ignoreStop)
	m := New(base)
	t.Cleanup(func() { _ = m.Close() })
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.serverDir, "server.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	return m
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tempo esgotado esperando: %s", what)
}

func TestStartRequiresEULA(t *testing.T) {
	m := newTestServer(t, false)
	if err := m.StartServer(context.Background()); !errors.Is(err, ErrEULANotAccepted) {
		t.Fatalf("esperado ErrEULANotAccepted, recebido %v", err)
	}
	if err := m.StartAll(context.Background()); !errors.Is(err, ErrEULANotAccepted) {
		t.Fatalf("StartAll deveria exigir a EULA antes de baixar arquivos: %v", err)
	}
}

func TestLifecycleStartCommandStop(t *testing.T) {
	m := newTestServer(t, false)
	if err := m.AcceptEULA(); err != nil {
		t.Fatal(err)
	}
	if err := m.StartServer(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "estado running", func() bool { return m.Status().ServerState == StateRunning })
	if err := m.SendCommand("say oi"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "eco do comando no log", func() bool { return strings.Contains(m.LogText(), "recebido say oi") })
	if err := m.StopServer(true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "processo encerrado", func() bool { return m.currentProcess() == nil })
	if state := m.Status().ServerState; state != StateStandby {
		t.Fatalf("estado após parada em standby = %s", state)
	}
	if !strings.Contains(m.LogText(), "Stopping server") {
		t.Fatal("a última linha do processo se perdeu antes do Wait")
	}
	data, err := os.ReadFile(filepath.Join(m.serverDir, "bukkit.yml"))
	if err != nil || !strings.Contains(string(data), "connection-throttle: 0") {
		t.Fatalf("bukkit.yml não foi preparado: %q %v", data, err)
	}
}

func TestStopWatchdogKillsUnresponsiveServer(t *testing.T) {
	previous := gracefulStopTimeout
	gracefulStopTimeout = 300 * time.Millisecond
	t.Cleanup(func() { gracefulStopTimeout = previous })
	m := newTestServer(t, true)
	if err := m.AcceptEULA(); err != nil {
		t.Fatal(err)
	}
	if err := m.StartServer(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "estado running", func() bool { return m.Status().ServerState == StateRunning })
	if err := m.StopServer(false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "finalização forçada", func() bool { return m.currentProcess() == nil })
	if !strings.Contains(m.LogText(), "finalizando à força") {
		t.Fatal("o vigia de parada deveria registrar a finalização forçada")
	}
}

func TestStartFailsFastDuringUpdate(t *testing.T) {
	m := newTestServer(t, false)
	if err := m.AcceptEULA(); err != nil {
		t.Fatal(err)
	}
	m.dependenciesMu.Lock()
	started := time.Now()
	err := m.StartServer(context.Background())
	m.dependenciesMu.Unlock()
	if !errors.Is(err, errUpdateInProgress) {
		t.Fatalf("esperado errUpdateInProgress, recebido %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("StartServer não deveria esperar a atualização terminar")
	}
}

func TestWriteTimesOutOnBlockedPipe(t *testing.T) {
	previous := stdinWriteTimeout
	stdinWriteTimeout = 200 * time.Millisecond
	t.Cleanup(func() { stdinWriteTimeout = previous })
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	proc := newJavaProcess(nil, writer)
	big := strings.Repeat("x", 4*1024*1024)
	if err := proc.Write(big); !errors.Is(err, errStdinBlocked) {
		t.Fatalf("esperado errStdinBlocked, recebido %v", err)
	}
	started := time.Now()
	if err := proc.Write("list\n"); !errors.Is(err, errStdinBlocked) {
		t.Fatalf("escritas seguintes deveriam falhar na hora: %v", err)
	}
	if time.Since(started) > 50*time.Millisecond {
		t.Fatal("escrita após travamento não deveria esperar o prazo de novo")
	}
}
