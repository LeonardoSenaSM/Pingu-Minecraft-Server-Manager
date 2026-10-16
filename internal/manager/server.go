package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mineserver/internal/api"
	"mineserver/internal/commands"
)

var errUpdateInProgress = errors.New("uma atualização de arquivos está em andamento; aguarde terminar para iniciar o servidor")

func (m *Manager) StartAll(ctx context.Context) error {
	if err := m.Initialize(); err != nil {
		return err
	}
	if err := m.ctx.Err(); err != nil {
		return err
	}
	if !m.EULAAccepted() {
		return ErrEULANotAccepted
	}
	if !m.dependenciesPresent() {
		m.Log("info", "dependencies", "dependências ausentes; iniciando download", nil)
		if err := m.UpdateDependencies(ctx); err != nil {
			return err
		}
	} else if selected := m.SelectedVersion(); selected != api.LatestVersion && selected != m.InstalledPaperVersion() {
		if err := m.UpdateDependencies(ctx); err != nil {
			return err
		}
	}
	ports, err := m.proxy.Start(m.ctx, javaPortStart, bedrockPortStart)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.ports = ports
	running := m.proc != nil
	m.mu.Unlock()
	if !running {
		m.setServerState(StateStandby)
	}
	m.notifyStatus()
	m.Log("info", "lifecycle", "automação sob demanda ativada", nil)
	return nil
}

// StartServer inicia a JVM. Não espera downloads: se uma atualização estiver
// em andamento, falha na hora para não deixar uma conexão de jogador presa.
func (m *Manager) StartServer(ctx context.Context) error {
	m.processMu.Lock()
	defer m.processMu.Unlock()
	if !m.dependenciesMu.TryLock() {
		return errUpdateInProgress
	}
	defer m.dependenciesMu.Unlock()
	m.mu.RLock()
	closed, running := m.closed, m.proc != nil
	m.mu.RUnlock()
	if closed {
		return context.Canceled
	}
	if running {
		return nil
	}
	if !m.EULAAccepted() {
		return ErrEULANotAccepted
	}
	jar := filepath.Join(m.serverDir, "server.jar")
	if info, err := os.Stat(jar); err != nil || info.Size() == 0 {
		return errors.New("server.jar ausente; atualize as dependências")
	}
	if err := m.prepareServerConfig(); err != nil {
		m.Log("warning", "config", "não foi possível ajustar bukkit.yml", err)
	}
	javaPath, javaFeature, err := m.findJavaExecutable()
	if err != nil {
		return err
	}
	plan, err := m.validatedMemoryPlan()
	if err != nil {
		return fmt.Errorf("configuração de memória inválida: %w", err)
	}
	args := append(plan.JVMArgs(), "-jar", "server.jar", "nogui")
	cmd := exec.Command(javaPath, args...)
	cmd.Dir = m.serverDir
	cmd.Env = append(os.Environ(), "JAVA_TOOL_OPTIONS=-Dfile.encoding=UTF-8")
	configureChildProcess(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		m.setServerState(StateStopped)
		return err
	}
	proc := newJavaProcess(cmd, stdin)
	proc.started = time.Now()
	m.mu.Lock()
	m.proc = proc
	m.serverState = StateStarting
	m.standbyOnExit = false
	m.mu.Unlock()
	m.notifyStatus()
	m.Log("info", "paper", fmt.Sprintf("iniciando PaperMC com Java %d (%s), modo de memória %s, RAM física %d GB, heap -Xms%dG/-Xmx%dG", javaFeature, javaPath, plan.Mode, plan.TotalGB, plan.InitialGB, plan.MaximumGB), nil)

	// cmd.Wait fecha os pipes; por isso só é chamado depois que as duas
	// leituras terminam. Sem isso, as últimas linhas (muitas vezes a causa de
	// uma queda) podiam se perder.
	var readers sync.WaitGroup
	readers.Add(2)
	go func() { defer readers.Done(); m.consumeProcessOutput("Paper/stdout", stdout) }()
	go func() { defer readers.Done(); m.consumeProcessOutput("Paper/stderr", stderr) }()
	m.goTracked(func(managerCtx context.Context) {
		select {
		case <-managerCtx.Done():
			if err := proc.Write("save-all\nstop\n"); err != nil && !errors.Is(err, errProcessExited) {
				m.Log("warning", "paper", "falha ao parar processo durante cancelamento", err)
			}
		case <-proc.Done():
		}
	})
	go func() {
		readers.Wait()
		waitErr := cmd.Wait()
		m.mu.Lock()
		if m.proc == proc {
			m.proc = nil
			if m.standbyOnExit {
				m.serverState = StateStandby
			} else {
				m.serverState = StateStopped
			}
			m.players = make(map[string]time.Time)
		}
		m.mu.Unlock()
		proc.markExited()
		if waitErr != nil {
			m.Log("error", "paper", describeProcessExit(waitErr), waitErr)
		} else {
			m.Log("info", "paper", "servidor encerrado", nil)
		}
		m.notifyStatus()
		m.notifyPlayers()
	}()
	return nil
}

// StopServer pede a parada graciosa e retorna. Um vigia finaliza a JVM se ela
// não sair dentro de gracefulStopTimeout.
func (m *Manager) StopServer(standby bool) error {
	m.processMu.Lock()
	m.mu.Lock()
	proc := m.proc
	if proc == nil {
		if standby {
			m.serverState = StateStandby
		} else {
			m.serverState = StateStopped
		}
		m.mu.Unlock()
		m.processMu.Unlock()
		m.notifyStatus()
		return nil
	}
	m.standbyOnExit = standby
	m.mu.Unlock()
	m.processMu.Unlock()

	err := proc.Write("save-all\nstop\n")
	switch {
	case err == nil, errors.Is(err, errProcessExited):
	case errors.Is(err, errStdinBlocked):
		m.Log("error", "paper", "o Paper não responde a comandos; finalizando à força", err)
		return proc.Kill()
	default:
		return fmt.Errorf("enviar parada ao Paper: %w", err)
	}
	m.goTracked(func(ctx context.Context) {
		timer := time.NewTimer(gracefulStopTimeout)
		defer timer.Stop()
		select {
		case <-proc.Done():
		case <-ctx.Done():
		case <-timer.C:
			m.Log("warning", "paper", fmt.Sprintf("Paper não encerrou em %s após \"stop\"; finalizando à força", gracefulStopTimeout), nil)
			if err := proc.Kill(); err != nil {
				m.Log("error", "paper", "falha ao finalizar o Paper", err)
			}
		}
	})
	return nil
}

func (m *Manager) RestartServer(ctx context.Context) error {
	proc := m.currentProcess()
	if proc == nil {
		return errors.New("o servidor não está em execução")
	}
	if err := m.StopServer(false); err != nil {
		return err
	}
	timer := time.NewTimer(gracefulStopTimeout + 20*time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return errors.New("tempo limite aguardando o encerramento do servidor")
	case <-proc.Done():
	}
	return m.StartServer(ctx)
}

// SendCommand envia um comando ao console do Paper sem segurar o lock global
// durante a escrita.
func (m *Manager) SendCommand(command string) error {
	if err := commands.ValidateInput(command); err != nil {
		return err
	}
	command = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(command), "/"))
	proc := m.currentProcess()
	if proc == nil {
		return errProcessExited
	}
	if err := proc.Write(command + "\n"); err != nil {
		return err
	}
	m.Log("info", "console", "> "+command, nil)
	return nil
}
