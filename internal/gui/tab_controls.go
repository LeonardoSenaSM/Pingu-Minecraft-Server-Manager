package gui

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/api"
)

// controlsTab: versão do Minecraft e comandos rápidos de mundo/jogador.
type controlsTab struct {
	u         *UI
	root      fyne.CanvasObject
	versions  *widget.Select
	installed *widget.Label
	player    *widget.Entry

	// thread da UI
	available  []string
	byDisplay  map[string]string
	populating bool
}

func newControlsTab(u *UI) *controlsTab {
	t := &controlsTab{u: u, available: []string{api.LatestVersion}, byDisplay: map[string]string{}}
	t.versions = widget.NewSelect(nil, t.onVersionSelected)
	t.installed = widget.NewLabel("")
	t.installed.Wrapping = fyne.TextWrapWord
	u.onTranslate(t.applyInstalled)
	u.onTranslate(t.renderVersions)

	apply := u.button("apply_version", theme.DownloadIcon(), t.applyVersion)
	refresh := u.button("refresh_list", theme.ViewRefreshIcon(), func() { go t.loadVersions() })
	versionCard := u.card("minecraft_version", "version_subtitle", container.NewVBox(
		t.versions, t.installed, container.NewGridWithColumns(2, apply, refresh),
		u.label("version_warning"), u.label("backups_note"),
	))

	t.player = u.entry("player_name")
	playerCommand := func(key, command string) *widget.Button {
		return u.button(key, nil, func() {
			name := strings.TrimSpace(t.player.Text)
			if name == "" {
				u.showInfo("name_required", "name_required_message")
				return
			}
			u.sendCommand("command_failed", command+" "+name)
		})
	}

	t.root = container.NewVScroll(container.NewVBox(
		versionCard,
		u.card("general", "general_subtitle", u.commandButton("save_world", "save-all")),
		u.card("weather", "weather_subtitle", container.NewGridWithColumns(3,
			u.commandButton("clear", "weather clear"), u.commandButton("rain", "weather rain"), u.commandButton("thunder", "weather thunder"))),
		u.card("time", "time_subtitle", container.NewGridWithColumns(2,
			u.commandButton("day", "time set day"), u.commandButton("night", "time set night"))),
		u.card("difficulty", "difficulty_subtitle", container.NewGridWithColumns(4,
			u.commandButton("peaceful", "difficulty peaceful"), u.commandButton("easy", "difficulty easy"),
			u.commandButton("normal", "difficulty normal"), u.commandButton("hard", "difficulty hard"))),
		u.card("gamemode", "gamemode_subtitle", container.NewVBox(t.player, container.NewGridWithColumns(2,
			playerCommand("survival", "gamemode survival"), playerCommand("creative", "gamemode creative")))),
	))
	return t
}

// loadVersions consulta o PaperMC fora da UI.
func (t *controlsTab) loadVersions() {
	versions, err := t.u.manager.AvailablePaperVersions(t.u.manager.Context())
	if err != nil {
		t.u.showError("versions_failed", err)
		return
	}
	t.u.do(func() {
		t.available = versions
		t.renderVersions()
	})
}

// renderVersions refaz as opções do seletor. Thread da UI.
func (t *controlsTab) renderVersions() {
	selected := t.u.manager.SelectedVersion()
	t.byDisplay = make(map[string]string, len(t.available))
	options := make([]string, 0, len(t.available))
	selectedDisplay := ""
	for _, version := range t.available {
		display := version
		if version == api.LatestVersion {
			display = t.u.loc.T("latest_stable")
		}
		t.byDisplay[display] = version
		options = append(options, display)
		if version == selected {
			selectedDisplay = display
		}
	}
	if selectedDisplay == "" && len(options) > 0 {
		selectedDisplay = options[0]
	}
	t.populating = true
	t.versions.Options = options
	t.versions.SetSelected(selectedDisplay)
	t.versions.Refresh()
	t.populating = false
}

func (t *controlsTab) onVersionSelected(display string) {
	if t.populating {
		return
	}
	version := t.byDisplay[display]
	if version == "" {
		return
	}
	m := t.u.manager
	if err := m.SetSelectedVersion(version); err != nil {
		t.u.showError("settings_failed", err)
		return
	}
	go func() {
		if err := m.SaveCurrentSettings(); err != nil {
			m.Log("error", "settings", "falha ao salvar versão selecionada", err)
		}
	}()
}

func (t *controlsTab) applyVersion() {
	u := t.u
	u.runOperation(func(ctx context.Context) error { return u.manager.UpdatePaperVersion(ctx) }, func(err error) {
		if err != nil {
			u.showError("apply_failed", err)
			return
		}
		u.refreshInstalledVersion()
		u.showInfo("version_applied", "version_ready", u.manager.InstalledPaperVersion())
	})
}

// applyInstalled mostra a versão instalada. Thread da UI.
func (t *controlsTab) applyInstalled() {
	loc := t.u.loc
	switch {
	case !t.u.installedLoaded:
		t.installed.SetText(loc.T("installed_version", loc.T("loading")))
	case t.u.installedVersion == "":
		t.installed.SetText(loc.T("installed_version", loc.T("unknown")))
	default:
		t.installed.SetText(loc.T("installed_version", t.u.installedVersion))
	}
}
