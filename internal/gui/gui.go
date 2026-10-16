package gui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/i18n"
	"mineserver/internal/manager"
)

// UI monta a janela a partir de abas independentes. Regras de thread:
//   - campos marcados "thread da UI" só são lidos/escritos dentro de fyne.Do
//     ou de callbacks de widgets;
//   - nada de rede, disco ou espera de processo roda na thread da UI.
type UI struct {
	manager *manager.Manager
	app     fyne.App
	window  fyne.Window
	loc     *i18n.Localizer

	// do agenda trabalho na thread da UI (fyne.Do). Testes trocam por uma
	// versão serializada, porque o driver de teste do Fyne executa fyne.Do na
	// goroutine que chama.
	do func(func())

	opMu     sync.Mutex
	opCancel context.CancelFunc
	busy     bool

	// thread da UI
	translators      []func()
	installedVersion string
	installedLoaded  bool
	lastStatus       manager.Status

	progress *progressPanel
	home     *homeTab
	console  *consoleTab
	players  *playersTab
	controls *controlsTab
	settings *settingsTab
	tabs     *container.AppTabs
}

func New(app fyne.App, window fyne.Window, service *manager.Manager) *UI {
	settings := service.Settings()
	return &UI{manager: service, app: app, window: window, loc: i18n.New(settings.Language), do: fyne.Do}
}

func (u *UI) Build() fyne.CanvasObject {
	u.progress = newProgressPanel(u)
	u.home = newHomeTab(u)
	u.console = newConsoleTab(u)
	u.players = newPlayersTab(u)
	u.controls = newControlsTab(u)
	u.settings = newSettingsTab(u)

	u.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon(u.loc.T("home_tab"), theme.HomeIcon(), u.home.root),
		container.NewTabItemWithIcon(u.loc.T("console_tab"), theme.ComputerIcon(), u.console.root),
		container.NewTabItemWithIcon(u.loc.T("players_tab"), theme.AccountIcon(), u.players.root),
		container.NewTabItemWithIcon(u.loc.T("controls_tab"), theme.SettingsIcon(), u.controls.root),
		container.NewTabItemWithIcon(u.loc.T("settings_tab"), theme.FolderIcon(), u.settings.root),
	)
	tabKeys := []string{"home_tab", "console_tab", "players_tab", "controls_tab", "settings_tab"}
	u.onTranslate(func() {
		for index, key := range tabKeys {
			u.tabs.Items[index].Text = u.loc.T(key)
		}
		u.tabs.Refresh()
	})

	u.manager.SetEvents(manager.Events{
		Log:     u.console.notify,
		Status:  func(status manager.Status) { u.do(func() { u.applyStatus(status) }) },
		Players: func() { go u.players.refresh() },
		Files:   func() { go u.settings.refreshFiles() },
		Language: func(language string) {
			u.do(func() { u.loc.SetLanguage(language); u.retranslate() })
		},
		Settings: func(saved manager.Settings) { u.do(func() { u.settings.apply(saved) }) },
		Progress: func(progress manager.Progress) { u.do(func() { u.progress.update(progress) }) },
	})
	u.retranslate()
	u.console.startPump(u.manager.Context())

	go func() {
		if err := u.manager.Initialize(); err != nil {
			u.manager.Log("error", "startup", "falha na inicialização", err)
			u.showError("start_failed", err)
		}
		u.refreshInstalledVersion()
	}()
	go u.controls.loadVersions()
	return u.tabs
}

// retranslate reaplica todos os textos. Thread da UI.
func (u *UI) retranslate() {
	for _, translate := range u.translators {
		translate()
	}
	u.applyStatus(u.lastStatus)
	go u.players.refresh()
	go u.settings.refreshFiles()
}

// applyStatus distribui o status às abas. Thread da UI.
func (u *UI) applyStatus(status manager.Status) {
	u.lastStatus = status
	u.home.applyStatus(status)
}

// refreshInstalledVersion lê a versão instalada fora da UI e atualiza quem a
// exibe.
func (u *UI) refreshInstalledVersion() {
	installed := u.manager.InstalledPaperVersion()
	status := u.manager.Status()
	u.do(func() {
		u.installedVersion = installed
		u.installedLoaded = true
		u.controls.applyInstalled()
		u.applyStatus(status)
	})
}

// onTranslate registra uma função chamada a cada troca de idioma.
func (u *UI) onTranslate(translate func()) {
	u.translators = append(u.translators, translate)
}

func (u *UI) showError(titleKey string, err error) {
	u.do(func() { dialog.ShowError(fmt.Errorf("%s: %w", u.loc.T(titleKey), err), u.window) })
}

func (u *UI) showInfo(titleKey, messageKey string, args ...any) {
	u.do(func() { dialog.ShowInformation(u.loc.T(titleKey), u.loc.T(messageKey, args...), u.window) })
}

// confirmSensitive pede confirmação antes de comandos de risco. Thread da UI.
func (u *UI) confirmSensitive(command string, confirmed func()) {
	dialog.ShowConfirm(u.loc.T("confirm_command_title"), u.loc.T("confirm_command_message", command), func(ok bool) {
		if ok {
			confirmed()
		}
	}, u.window)
}

// sendCommand envia um comando em segundo plano e mostra erro se falhar.
func (u *UI) sendCommand(errorKey, command string) {
	u.runSimple(errorKey, func(context.Context) error { return u.manager.SendCommand(command) })
}

// requireEULA mostra o aceite da EULA quando a operação falhou por falta
// dele, e repete a operação se o usuário aceitar. Retorna true se tratou o erro.
func (u *UI) requireEULA(err error, retry func()) bool {
	if !errors.Is(err, manager.ErrEULANotAccepted) {
		return false
	}
	u.do(func() {
		message := widget.NewLabel(u.loc.T("eula_message", manager.EULAURL))
		message.Wrapping = fyne.TextWrapWord
		link := widget.NewHyperlink(u.loc.T("eula_open"), mustParseURL(manager.EULAURL))
		content := container.NewVBox(message, link)
		confirm := dialog.NewCustomConfirm(u.loc.T("eula_title"), u.loc.T("eula_accept"), u.loc.T("cancel"), content, func(accepted bool) {
			if !accepted {
				u.showInfo("eula_title", "eula_declined")
				return
			}
			go func() {
				if err := u.manager.AcceptEULA(); err != nil {
					u.showError("start_failed", err)
					return
				}
				retry()
			}()
		}, u.window)
		confirm.Resize(fyne.NewSize(520, 260))
		confirm.Show()
	})
	return true
}

func (u *UI) runSimple(errorKey string, operation func(context.Context) error) {
	go func() {
		if err := operation(u.manager.Context()); err != nil {
			u.showError(errorKey, err)
		}
	}()
}

// runOperation executa uma operação longa e cancelável (download, início).
// Só uma roda por vez.
func (u *UI) runOperation(operation func(context.Context) error, completed func(error)) {
	u.opMu.Lock()
	if u.busy {
		u.opMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(u.manager.Context())
	u.busy = true
	u.opCancel = cancel
	u.opMu.Unlock()
	u.do(u.progress.cancel.Enable)
	go func() {
		err := operation(ctx)
		cancel()
		u.opMu.Lock()
		u.busy = false
		u.opCancel = nil
		u.opMu.Unlock()
		u.do(u.progress.cancel.Disable)
		completed(err)
	}()
}

func (u *UI) cancelOperation() {
	u.opMu.Lock()
	cancel := u.opCancel
	u.opMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func mustParseURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return parsed
}

func openFolder(path string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

func formatDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	totalSeconds := int64(duration / time.Second)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}
