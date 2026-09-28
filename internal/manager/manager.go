package manager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"mineserver/internal/api"
	"mineserver/internal/i18n"
	"mineserver/internal/proxy"
)

const (
	javaPortStart    = 25565
	bedrockPortStart = 19132
	idleTimeout      = 15 * time.Minute
	maxLogEntries    = 2500
)

type Manager struct {
	baseDir    string
	serverDir  string
	pluginsDir string
	api        *api.Client
	proxy      *proxy.Service

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu            sync.RWMutex
	events        Events
	settings      Settings
	serverState   string
	ports         proxy.Ports
	serverCmd     *exec.Cmd
	serverStdin   io.WriteCloser
	serverDone    chan struct{}
	serverStarted time.Time
	standbyOnExit bool
	players       map[string]time.Time
	knownPlayers  map[string]time.Time
	managedBans   []BanEntry
	logs          []LogEntry
	logStart      int
	closed        bool
	totalMemoryGB int
	memoryError   error

	processMu      sync.Mutex
	dependenciesMu sync.Mutex
	settingsMu     sync.Mutex
	closeOnce      sync.Once
	closeErr       error
	initializeOnce sync.Once
	initializeErr  error
}

func New(baseDir string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	totalMemoryGB, memoryErr := detectPhysicalMemoryGB()
	if totalMemoryGB < 1 {
		totalMemoryGB = 4
	}
	m := &Manager{
		baseDir:    baseDir,
		serverDir:  filepath.Join(baseDir, "server"),
		pluginsDir: filepath.Join(baseDir, "server", "plugins"),
		api:        api.NewClient(10 * time.Minute),
		ctx:        ctx,
		cancel:     cancel,
		settings: Settings{
			SelectedMinecraftVersion: api.LatestVersion,
			Language:                 i18n.Portuguese,
			ServerName:               "Pingu",
			MaxPlayers:               20,
			MemoryMode:               MemoryModeAutomatic,
		},
		serverState:   StateStopped,
		totalMemoryGB: totalMemoryGB,
		memoryError:   memoryErr,
		players:       make(map[string]time.Time),
		knownPlayers:  make(map[string]time.Time),
	}
	m.proxy = proxy.New(idleTimeout, proxy.Hooks{
		Prepare: m.prepareProxyBackends,
		Wake: func(ctx context.Context) error {
			return m.StartServer(ctx)
		},
		CanIdle: func() bool {
			m.mu.RLock()
			defer m.mu.RUnlock()
			return m.serverCmd != nil && len(m.players) == 0
		},
		OnIdle: func() {
			m.Log("info", "lifecycle", "15 minutos sem jogadores ou tráfego; entrando em espera", nil)
			if err := m.StopServer(true); err != nil {
				m.Log("error", "lifecycle", "falha ao entrar em espera", err)
			}
		},
		Log: func(level, message string, err error) {
			m.Log(level, "proxy", message, err)
		},
	})
	return m
}

func (m *Manager) Context() context.Context { return m.ctx }
func (m *Manager) ServerDir() string        { return m.serverDir }

func (m *Manager) SetEvents(events Events) {
	m.mu.Lock()
	m.events = events
	m.mu.Unlock()
}

func (m *Manager) Initialize() error {
	m.initializeOnce.Do(func() {
		if err := m.ensureDirectories(); err != nil {
			m.initializeErr = err
			return
		}
		m.loadSettings()
		if m.memoryError != nil {
			m.Log("warning", "memory", "não foi possível detectar a RAM física; usando 4 GB como referência segura", m.memoryError)
		}
		if err := m.updateServerProperties(m.paperPort()); err != nil {
			m.initializeErr = err
			return
		}
		m.loadManagedBans()
		m.loadKnownPlayers()
		m.goTracked(m.banExpiryLoop)
		m.goTracked(m.statusMonitorLoop)
		m.notifyStatus()
		m.notifyPlayers()
		m.notifyFiles()
	})
	return m.initializeErr
}

func (m *Manager) Settings() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

func (m *Manager) SaveSettings(language, serverName string, maxPlayers int) error {
	settings := m.Settings()
	return m.SaveSettingsWithMemory(language, serverName, maxPlayers, settings.MemoryMode, settings.MemoryLimitGB)
}

func (m *Manager) SaveSettingsWithMemory(language, serverName string, maxPlayers int, memoryMode string, memoryLimitGB int) error {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	serverName = strings.TrimSpace(serverName)
	if language != i18n.English && language != i18n.Portuguese {
		language = i18n.Portuguese
	}
	if serverName == "" || maxPlayers < 1 || maxPlayers > 1000 {
		return errors.New("nome inválido ou max-players fora do intervalo de 1 a 1000")
	}
	if _, err := BuildMemoryPlan(m.totalMemoryGB, memoryMode, memoryLimitGB); err != nil {
		return err
	}
	m.mu.Lock()
	m.settings.Language = language
	m.settings.ServerName = serverName
	m.settings.MaxPlayers = maxPlayers
	m.settings.MemoryMode = memoryMode
	m.settings.MemoryLimitGB = memoryLimitGB
	settings := m.settings
	port := m.paperPortLocked()
	m.mu.Unlock()
	if err := writeJSONAtomic(m.settingsPath(), settings); err != nil {
		return fmt.Errorf("salvar config.json: %w", err)
	}
	if err := m.updateServerProperties(port); err != nil {
		return err
	}
	if info, err := os.Stat(filepath.Join(m.pluginsDir, "Geyser-Spigot.jar")); err == nil && info.Size() > 0 {
		if err := m.configureGeyser(m.geyserPort(), port); err != nil {
			return err
		}
	}
	m.notifyLanguage(language)
	m.notifySettings()
	m.notifyFiles()
	return nil
}

func (m *Manager) SaveCurrentSettings() error {
	m.settingsMu.Lock()
	defer m.settingsMu.Unlock()
	m.mu.RLock()
	settings := m.settings
	m.mu.RUnlock()
	return writeJSONAtomic(m.settingsPath(), settings)
}

func (m *Manager) SetLanguage(language string) {
	if language != i18n.English && language != i18n.Portuguese {
		language = i18n.Portuguese
	}
	m.mu.Lock()
	m.settings.Language = language
	m.mu.Unlock()
}

func (m *Manager) SetSelectedVersion(version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return errors.New("versão vazia")
	}
	m.mu.Lock()
	m.settings.SelectedMinecraftVersion = version
	m.mu.Unlock()
	return nil
}

func (m *Manager) SelectedVersion() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.settings.SelectedMinecraftVersion == "" {
		return api.LatestVersion
	}
	return m.settings.SelectedMinecraftVersion
}

func (m *Manager) Status() Status {
	health := m.proxy.Health()
	ports := health.Ports
	m.mu.RLock()
	state := m.serverState
	settings := m.settings
	playersOnline := len(m.players)
	started := m.serverStarted
	m.mu.RUnlock()
	plan, planErr := BuildMemoryPlan(m.totalMemoryGB, settings.MemoryMode, settings.MemoryLimitGB)
	if planErr != nil {
		plan, _ = BuildMemoryPlan(m.totalMemoryGB, MemoryModeAutomatic, 0)
	}
	uptime := time.Duration(0)
	if !started.IsZero() {
		uptime = time.Since(started).Truncate(time.Second)
	}
	return Status{
		ServerState:   state,
		NetworkActive: health.TCPListening && health.UDPListening,
		JavaListening: health.TCPListening, BedrockListening: health.UDPListening,
		JavaPort:    ports.JavaPublic,
		BedrockPort: ports.BedrockPublic, LocalIPs: ports.LocalIPs,
		ActiveSessions: m.proxy.SessionCount(),
		ServerName:     settings.ServerName, MinecraftVersion: m.InstalledPaperVersion(),
		PlayersOnline: playersOnline, MemoryMode: plan.Mode, TotalMemoryGB: plan.TotalGB,
		MemoryMaximumGB: plan.MaximumGB, Uptime: uptime,
	}
}

func (m *Manager) MemoryPlan() MemoryPlan {
	plan, err := m.validatedMemoryPlan()
	if err == nil {
		return plan
	}
	plan, _ = BuildMemoryPlan(m.totalMemoryGB, MemoryModeAutomatic, 0)
	return plan
}

func (m *Manager) validatedMemoryPlan() (MemoryPlan, error) {
	m.mu.RLock()
	settings := m.settings
	m.mu.RUnlock()
	return BuildMemoryPlan(m.totalMemoryGB, settings.MemoryMode, settings.MemoryLimitGB)
}

// ShutdownLinks atomically stops accepting new Java/Bedrock traffic, cancels
// all proxy sessions and waits for their tracked goroutines to finish.
func (m *Manager) ShutdownLinks() error {
	if err := m.proxy.Close(); err != nil {
		m.Log("error", "proxy", "falha ao derrubar links de conexão", err)
		m.notifyStatus()
		return err
	}
	m.Log("info", "proxy", "links Java e Bedrock desligados; portas liberadas", nil)
	m.notifyStatus()
	return nil
}

func (m *Manager) ConnectionText() string {
	status := m.Status()
	if !status.NetworkActive || len(status.LocalIPs) == 0 {
		return ""
	}
	lines := make([]string, 0, len(status.LocalIPs)*2)
	for _, address := range status.LocalIPs {
		lines = append(lines,
			fmt.Sprintf("Java: %s:%d (TCP)", address, status.JavaPort),
			fmt.Sprintf("Bedrock: %s:%d (UDP)", address, status.BedrockPort),
		)
	}
	return strings.Join(lines, "\n")
}

func (m *Manager) Log(level, component, message string, err error) {
	m.mu.Lock()
	entry := LogEntry{
		Time: time.Now(), Level: strings.ToUpper(level), Component: component,
		ServerName: m.settings.ServerName, Message: message,
	}
	if entry.ServerName == "" {
		entry.ServerName = "Pingu"
	}
	if err != nil {
		entry.Err = err.Error()
	}
	if len(m.logs) < maxLogEntries {
		m.logs = append(m.logs, entry)
	} else {
		m.logs[m.logStart] = entry
		m.logStart = (m.logStart + 1) % maxLogEntries
	}
	callback := m.events.Log
	m.mu.Unlock()
	if callback != nil {
		callback(entry)
	}
}

func (m *Manager) LogText() string {
	m.mu.RLock()
	lines := make([]string, len(m.logs))
	for index := range m.logs {
		entryIndex := (m.logStart + index) % len(m.logs)
		lines[index] = m.logs[entryIndex].String()
	}
	m.mu.RUnlock()
	return strings.Join(lines, "\n")
}

func (m *Manager) OnlinePlayers() []string {
	m.mu.RLock()
	result := make([]string, 0, len(m.players))
	for name := range m.players {
		result = append(result, name)
	}
	m.mu.RUnlock()
	sort.Strings(result)
	return result
}

func (m *Manager) OfflinePlayers() []string {
	m.mu.RLock()
	result := make([]string, 0, len(m.knownPlayers))
	for name := range m.knownPlayers {
		if _, online := m.players[name]; !online {
			result = append(result, name)
		}
	}
	m.mu.RUnlock()
	sort.Strings(result)
	return result
}

func (m *Manager) setServerState(state string) {
	m.mu.Lock()
	m.serverState = state
	m.mu.Unlock()
	m.notifyStatus()
}

func (m *Manager) paperPort() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.paperPortLocked()
}

func (m *Manager) paperPortLocked() int {
	if m.ports.PaperInternal > 0 {
		return m.ports.PaperInternal
	}
	return javaPortStart + 1
}

func (m *Manager) geyserPort() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.ports.GeyserInternal > 0 {
		return m.ports.GeyserInternal
	}
	return bedrockPortStart + 1
}

func (m *Manager) prepareProxyBackends(ports proxy.Ports) error {
	if err := m.updateServerProperties(ports.PaperInternal); err != nil {
		return err
	}
	if err := m.configureGeyser(ports.GeyserInternal, ports.PaperInternal); err != nil {
		return err
	}
	m.mu.Lock()
	m.ports = ports
	m.mu.Unlock()
	return nil
}

func (m *Manager) goTracked(task func(context.Context)) bool {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return false
	}
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		task(m.ctx)
	}()
	return true
}

func (m *Manager) statusMonitorLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.notifyStatus()
		}
	}
}

func (m *Manager) notifyStatus() {
	status := m.Status()
	m.mu.RLock()
	callback := m.events.Status
	m.mu.RUnlock()
	if callback != nil {
		callback(status)
	}
}

func (m *Manager) notifyPlayers() {
	m.mu.RLock()
	callback := m.events.Players
	m.mu.RUnlock()
	if callback != nil {
		callback()
	}
}

func (m *Manager) notifyFiles() {
	m.mu.RLock()
	callback := m.events.Files
	m.mu.RUnlock()
	if callback != nil {
		callback()
	}
}

func (m *Manager) notifyLanguage(language string) {
	m.mu.RLock()
	callback := m.events.Language
	m.mu.RUnlock()
	if callback != nil {
		callback(language)
	}
}

func (m *Manager) notifySettings() {
	m.mu.RLock()
	callback := m.events.Settings
	settings := m.settings
	m.mu.RUnlock()
	if callback != nil {
		callback(settings)
	}
}

func (m *Manager) notifyProgress(progress Progress) {
	m.mu.RLock()
	callback := m.events.Progress
	m.mu.RUnlock()
	if callback != nil {
		callback(progress)
	}
}

func (m *Manager) Close() error {
	m.closeOnce.Do(func() {
		m.processMu.Lock()
		m.mu.Lock()
		m.closed = true
		m.mu.Unlock()
		m.processMu.Unlock()
		proxyErr := m.proxy.Close()
		serverErr := m.shutdownProcess(12 * time.Second)
		m.cancel()
		m.wg.Wait()
		m.closeErr = errors.Join(proxyErr, serverErr)
	})
	return m.closeErr
}

func (m *Manager) shutdownProcess(timeout time.Duration) error {
	m.processMu.Lock()
	m.mu.RLock()
	cmd := m.serverCmd
	stdin := m.serverStdin
	done := m.serverDone
	m.mu.RUnlock()
	if cmd == nil {
		m.processMu.Unlock()
		return nil
	}
	if stdin != nil {
		if _, err := io.WriteString(stdin, "save-all\nstop\n"); err != nil {
			m.Log("warning", "paper", "falha ao enviar parada graciosa", err)
		}
	}
	m.processMu.Unlock()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		if cmd.Process != nil {
			if err := cmd.Process.Kill(); err != nil {
				return fmt.Errorf("forçar encerramento do Paper: %w", err)
			}
		}
		return errors.New("Paper não encerrou no prazo e foi finalizado")
	}
}
