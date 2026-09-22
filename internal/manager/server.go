package manager

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"mineserver/internal/api"
)

var (
	joinPattern  = regexp.MustCompile(`:\s+([.A-Za-z0-9_]{1,32}) joined the game`)
	leavePattern = regexp.MustCompile(`:\s+([.A-Za-z0-9_]{1,32}) left the game`)
)

func (m *Manager) StartAll(ctx context.Context) error {
	if err := m.Initialize(); err != nil {
		return err
	}
	if err := m.ctx.Err(); err != nil {
		return err
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
	running := m.serverCmd != nil
	m.mu.Unlock()
	if !running {
		m.setServerState(StateStandby)
	}
	m.notifyStatus()
	m.Log("info", "lifecycle", "automação sob demanda ativada", nil)
	return nil
}

func (m *Manager) dependenciesPresent() bool {
	paths := []string{
		filepath.Join(m.serverDir, "server.jar"),
		filepath.Join(m.pluginsDir, "Geyser-Spigot.jar"),
		filepath.Join(m.pluginsDir, "Floodgate-Spigot.jar"),
		filepath.Join(m.pluginsDir, "Chunky.jar"),
		filepath.Join(m.pluginsDir, "CoreProtect.jar"),
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			return false
		}
	}
	return true
}

func (m *Manager) AvailablePaperVersions(ctx context.Context) ([]string, error) {
	return m.api.PaperVersions(ctx)
}

func (m *Manager) UpdateDependencies(ctx context.Context) (result error) {
	m.dependenciesMu.Lock()
	defer m.dependenciesMu.Unlock()
	defer func() {
		progress := Progress{Done: true, TotalTasks: 5}
		if result != nil {
			progress.Err = result.Error()
		}
		m.notifyProgress(progress)
	}()
	if ctx == nil {
		ctx = m.ctx
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	m.mu.RLock()
	running := m.serverCmd != nil
	m.mu.RUnlock()
	if running {
		return errors.New("pare o servidor antes de atualizar os arquivos")
	}
	if err := m.ensureDirectories(); err != nil {
		return err
	}
	m.notifyProgress(Progress{Task: "PaperMC", TotalTasks: 5, Indeterminate: true})
	paperURL, paperVersion, err := m.api.ResolvePaper(ctx, m.SelectedVersion())
	if err != nil {
		return err
	}
	type task struct {
		name string
		path string
		url  func(context.Context) (string, error)
	}
	tasks := []task{
		{"PaperMC", filepath.Join(m.serverDir, "server.jar"), func(context.Context) (string, error) { return paperURL, nil }},
		{"Geyser-Spigot", filepath.Join(m.pluginsDir, "Geyser-Spigot.jar"), func(context.Context) (string, error) {
			return "https://download.geysermc.org/v2/projects/geyser/versions/latest/builds/latest/downloads/spigot", nil
		}},
		{"Floodgate-Spigot", filepath.Join(m.pluginsDir, "Floodgate-Spigot.jar"), func(context.Context) (string, error) {
			return "https://download.geysermc.org/v2/projects/floodgate/versions/latest/builds/latest/downloads/spigot", nil
		}},
		{"Chunky", filepath.Join(m.pluginsDir, "Chunky.jar"), func(ctx context.Context) (string, error) {
			return m.api.ModrinthDownload(ctx, "chunky", paperVersion)
		}},
		{"CoreProtect", filepath.Join(m.pluginsDir, "CoreProtect.jar"), func(ctx context.Context) (string, error) {
			return m.api.ModrinthDownload(ctx, "coreprotect", paperVersion)
		}},
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, len(tasks))
	var wait sync.WaitGroup
	var completedMu sync.Mutex
	completed := 0
	for _, item := range tasks {
		item := item
		wait.Add(1)
		go func() {
			defer wait.Done()
			downloadURL, err := item.url(ctx)
			if err != nil {
				errCh <- fmt.Errorf("localizar %s: %w", item.name, err)
				cancel()
				return
			}
			m.Log("info", "download", "baixando "+item.name, nil)
			err = m.api.Download(ctx, downloadURL, item.path, func(downloaded, total int64) {
				completedMu.Lock()
				current := completed
				completedMu.Unlock()
				m.notifyProgress(Progress{Task: item.name, Downloaded: downloaded, Total: total, Completed: current, TotalTasks: len(tasks), Indeterminate: total <= 0})
			})
			if err != nil {
				errCh <- fmt.Errorf("baixar %s: %w", item.name, err)
				cancel()
				return
			}
			completedMu.Lock()
			completed++
			current := completed
			completedMu.Unlock()
			m.notifyProgress(Progress{Task: item.name, Completed: current, TotalTasks: len(tasks)})
		}()
	}
	wait.Wait()
	close(errCh)
	var downloadErrors []error
	for err := range errCh {
		downloadErrors = append(downloadErrors, err)
	}
	if len(downloadErrors) > 0 {
		return errors.Join(downloadErrors...)
	}
	if err := writeJSONAtomic(m.paperInstallationPath(), PaperInstallation{Version: paperVersion, DownloadedAt: time.Now()}); err != nil {
		return err
	}
	if err := m.configureGeyser(m.geyserPort(), m.paperPort()); err != nil {
		return err
	}
	m.Log("info", "dependencies", "todas as dependências foram atualizadas", nil)
	m.notifyFiles()
	return nil
}

func (m *Manager) UpdatePaperVersion(ctx context.Context) (result error) {
	m.dependenciesMu.Lock()
	defer m.dependenciesMu.Unlock()
	defer func() {
		progress := Progress{Done: true, TotalTasks: 1}
		if result != nil {
			progress.Err = result.Error()
		}
		m.notifyProgress(progress)
	}()
	m.mu.RLock()
	running := m.serverCmd != nil
	m.mu.RUnlock()
	if running {
		return errors.New("pare o servidor antes de trocar a versão")
	}
	if err := m.ensureDirectories(); err != nil {
		return err
	}
	downloadURL, version, err := m.api.ResolvePaper(ctx, m.SelectedVersion())
	if err != nil {
		return err
	}
	destination := filepath.Join(m.serverDir, "server.jar")
	if err := m.api.Download(ctx, downloadURL, destination, func(downloaded, total int64) {
		m.notifyProgress(Progress{Task: "PaperMC " + version, Downloaded: downloaded, Total: total, TotalTasks: 1, Indeterminate: total <= 0})
	}); err != nil {
		return err
	}
	if err := writeJSONAtomic(m.paperInstallationPath(), PaperInstallation{Version: version, DownloadedAt: time.Now()}); err != nil {
		return err
	}
	m.Log("info", "dependencies", "PaperMC "+version+" instalado", nil)
	m.notifyFiles()
	return nil
}

func (m *Manager) StartServer(ctx context.Context) error {
	m.processMu.Lock()
	defer m.processMu.Unlock()
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return context.Canceled
	}
	if m.serverCmd != nil {
		m.mu.RUnlock()
		return nil
	}
	m.mu.RUnlock()
	jar := filepath.Join(m.serverDir, "server.jar")
	if info, err := os.Stat(jar); err != nil || info.Size() == 0 {
		return errors.New("server.jar ausente; atualize as dependências")
	}
	javaPath, err := exec.LookPath("java")
	if err != nil {
		return errors.New("Java não encontrado no PATH; instale Java 21 ou superior")
	}
	cmd := exec.Command(javaPath, "-Xms1G", "-Xmx4G", "-jar", "server.jar", "nogui")
	cmd.Dir = m.serverDir
	cmd.Env = append(os.Environ(), "JAVA_TOOL_OPTIONS=-Dfile.encoding=UTF-8")
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
	done := make(chan struct{})
	m.mu.Lock()
	m.serverCmd = cmd
	m.serverStdin = stdin
	m.serverDone = done
	m.serverState = StateStarting
	m.standbyOnExit = false
	m.mu.Unlock()
	m.notifyStatus()
	if err := cmd.Start(); err != nil {
		m.mu.Lock()
		m.serverCmd = nil
		m.serverStdin = nil
		m.serverDone = nil
		m.serverState = StateStopped
		m.mu.Unlock()
		close(done)
		m.notifyStatus()
		return err
	}
	m.Log("info", "paper", "iniciando PaperMC com 1–4 GB de RAM", nil)
	m.goTracked(func(context.Context) { m.consumeProcessOutput("Paper/stdout", stdout) })
	m.goTracked(func(context.Context) { m.consumeProcessOutput("Paper/stderr", stderr) })
	m.goTracked(func(managerCtx context.Context) {
		select {
		case <-managerCtx.Done():
			m.mu.RLock()
			current := m.serverCmd == cmd
			processStdin := m.serverStdin
			m.mu.RUnlock()
			if current && processStdin != nil {
				if _, writeErr := io.WriteString(processStdin, "save-all\nstop\n"); writeErr != nil {
					m.Log("warning", "paper", "falha ao parar processo durante cancelamento", writeErr)
				}
			}
		case <-done:
		}
	})
	m.goTracked(func(context.Context) {
		waitErr := cmd.Wait()
		m.mu.Lock()
		if m.serverCmd == cmd {
			m.serverCmd = nil
			m.serverStdin = nil
			m.serverDone = nil
			if m.standbyOnExit {
				m.serverState = StateStandby
			} else {
				m.serverState = StateStopped
			}
			m.players = make(map[string]time.Time)
		}
		m.mu.Unlock()
		close(done)
		if waitErr != nil {
			m.Log("error", "paper", "processo encerrado com erro", waitErr)
		} else {
			m.Log("info", "paper", "servidor encerrado", nil)
		}
		m.notifyStatus()
		m.notifyPlayers()
	})
	return nil
}

func (m *Manager) StopServer(standby bool) error {
	m.processMu.Lock()
	defer m.processMu.Unlock()
	m.mu.Lock()
	if m.serverCmd == nil {
		if standby {
			m.serverState = StateStandby
		} else {
			m.serverState = StateStopped
		}
		m.mu.Unlock()
		m.notifyStatus()
		return nil
	}
	m.standbyOnExit = standby
	stdin := m.serverStdin
	m.mu.Unlock()
	if stdin == nil {
		return errors.New("entrada do processo indisponível")
	}
	if _, err := io.WriteString(stdin, "save-all\nstop\n"); err != nil {
		return fmt.Errorf("enviar parada ao Paper: %w", err)
	}
	return nil
}

func (m *Manager) SendCommand(command string) error {
	command = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(command), "/"))
	if command == "" {
		return errors.New("comando vazio")
	}
	m.mu.Lock()
	stdin := m.serverStdin
	if stdin == nil {
		m.mu.Unlock()
		return errors.New("o servidor não está em execução")
	}
	_, err := io.WriteString(stdin, command+"\n")
	m.mu.Unlock()
	if err != nil {
		return err
	}
	m.Log("info", "console", "> "+command, nil)
	return nil
}

func (m *Manager) consumeProcessOutput(source string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		m.Log("info", source, line, nil)
		m.parseServerLine(line)
	}
	if err := scanner.Err(); err != nil {
		m.Log("error", source, "falha ao ler saída do processo", err)
	}
}

func (m *Manager) parseServerLine(line string) {
	if strings.Contains(line, "Done (") && strings.Contains(line, "For help, type") {
		m.setServerState(StateRunning)
	}
	if match := joinPattern.FindStringSubmatch(line); len(match) == 2 {
		m.mu.Lock()
		m.players[match[1]] = time.Now()
		m.knownPlayers[match[1]] = time.Now()
		m.mu.Unlock()
		m.notifyPlayers()
	}
	if match := leavePattern.FindStringSubmatch(line); len(match) == 2 {
		m.mu.Lock()
		delete(m.players, match[1])
		m.mu.Unlock()
		m.notifyPlayers()
	}
}

func (m *Manager) TempBan(name string, duration time.Duration) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("nome de jogador vazio")
	}
	if err := m.SendCommand("ban " + name + " Banimento temporário de 3 dias"); err != nil {
		return err
	}
	entry := BanEntry{Name: name, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(duration), Reason: "Banimento temporário de 3 dias"}
	m.mu.Lock()
	filtered := m.managedBans[:0]
	for _, previous := range m.managedBans {
		if !strings.EqualFold(previous.Name, name) {
			filtered = append(filtered, previous)
		}
	}
	m.managedBans = append(filtered, entry)
	m.mu.Unlock()
	if err := m.saveManagedBans(); err != nil {
		m.Log("warning", "storage", "não foi possível persistir o banimento", err)
	}
	m.notifyPlayers()
	return nil
}

func (m *Manager) Unban(entry BanEntry) error {
	command := ""
	if entry.Name != "" {
		command = "pardon " + entry.Name
	} else if entry.IP != "" {
		command = "pardon-ip " + entry.IP
	} else {
		return errors.New("registro de banimento inválido")
	}
	if err := m.SendCommand(command); err != nil {
		return err
	}
	m.mu.Lock()
	filtered := m.managedBans[:0]
	for _, previous := range m.managedBans {
		if !(strings.EqualFold(previous.Name, entry.Name) && previous.IP == entry.IP) {
			filtered = append(filtered, previous)
		}
	}
	m.managedBans = filtered
	m.mu.Unlock()
	if err := m.saveManagedBans(); err != nil {
		m.Log("warning", "storage", "não foi possível persistir o desbanimento", err)
	}
	m.notifyPlayers()
	return nil
}

func (m *Manager) banExpiryLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.mu.RLock()
			bans := append([]BanEntry(nil), m.managedBans...)
			running := m.serverCmd != nil
			m.mu.RUnlock()
			if !running {
				continue
			}
			for _, entry := range bans {
				if !entry.ExpiresAt.IsZero() && !now.Before(entry.ExpiresAt) {
					if err := m.Unban(entry); err != nil {
						m.Log("error", "players", "falha ao encerrar banimento de "+entry.Name, err)
					}
				}
			}
		}
	}
}
