package gui

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/manager"
)

// homeTab: visão geral, cartões de conexão e botões principais.
type homeTab struct {
	u             *UI
	root          fyne.CanvasObject
	serverStatus  *widget.Label
	networkStatus *widget.Label
	overview      *widget.Label
	links         *LinkControls
}

func newHomeTab(u *UI) *homeTab {
	t := &homeTab{u: u}
	t.serverStatus = widget.NewLabelWithStyle(u.loc.T(manager.StateStopped), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	t.networkStatus = widget.NewLabelWithStyle(u.loc.T("inactive"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	t.overview = widget.NewLabel("")
	t.overview.Wrapping = fyne.TextWrapWord
	t.links = BuildLinkControls(u.window, u.loc, u.manager.ShutdownLinks)
	u.onTranslate(t.links.Retranslate)

	play := u.button("play", theme.MediaPlayIcon(), t.play)
	play.Importance = widget.HighImportance
	stop := u.button("stop_server", theme.MediaStopIcon(), func() {
		u.confirmSensitive("stop", func() {
			u.runSimple("stop_failed", func(context.Context) error { return u.manager.StopServer(false) })
		})
	})
	stop.Importance = widget.DangerImportance
	restart := u.button("restart_server", theme.ViewRefreshIcon(), func() {
		u.runSimple("restart_failed", u.manager.RestartServer)
	})
	update := u.button("update_dependencies", theme.DownloadIcon(), t.updateDependencies)

	jvmCard := u.card("jvm_title", "jvm_subtitle", t.serverStatus)
	networkCard := u.card("network_title", "network_subtitle", t.networkStatus)
	overviewCard := u.card("overview_title", "overview_subtitle", t.overview)
	t.root = container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle("Pingu", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		play, overviewCard, t.links.Root, container.NewGridWithColumns(2, jvmCard, networkCard),
		container.NewGridWithColumns(3, stop, restart, update), u.progress.root, u.label("quick_guide"),
	))
	return t
}

func (t *homeTab) play() {
	u := t.u
	u.runOperation(func(ctx context.Context) error { return u.manager.StartAll(ctx) }, func(err error) {
		if err == nil || u.handleStartError(err, t.play) {
			return
		}
		u.showError("start_failed", err)
	})
}

func (t *homeTab) updateDependencies() {
	u := t.u
	u.runOperation(func(ctx context.Context) error { return u.manager.UpdateDependencies(ctx) }, func(err error) {
		if err != nil && u.handleStartError(err, t.updateDependencies) {
			return
		}
		if err != nil {
			u.showError("update_failed", err)
			return
		}
		u.refreshInstalledVersion()
		u.showInfo("updated_title", "updated_message")
	})
}

// applyStatus roda na thread da UI.
func (t *homeTab) applyStatus(status manager.Status) {
	loc := t.u.loc
	state := status.ServerState
	if state == "" {
		state = manager.StateStopped
	}
	t.serverStatus.SetText(loc.T(state))
	if status.NetworkActive {
		t.networkStatus.SetText(loc.T("listening"))
	} else {
		t.networkStatus.SetText(loc.T("inactive"))
	}
	javaAddress, bedrockAddress := loc.T("inactive"), loc.T("inactive")
	if len(status.LocalIPs) > 0 && status.JavaListening {
		javaAddress = fmt.Sprintf("%s:%d", status.LocalIPs[0], status.JavaPort)
	}
	if len(status.LocalIPs) > 0 && status.BedrockListening {
		bedrockAddress = fmt.Sprintf("%s:%d", status.LocalIPs[0], status.BedrockPort)
	}
	memoryModeKey := "memory_auto"
	if status.MemoryMode == manager.MemoryModeManual {
		memoryModeKey = "memory_manual"
	}
	minecraftVersion := status.MinecraftVersion
	if minecraftVersion == "" {
		minecraftVersion = loc.T("unknown")
	}
	t.overview.SetText(strings.Join([]string{
		loc.T("overview_server", status.ServerName),
		loc.T("overview_version", minecraftVersion),
		loc.T("overview_java", javaAddress),
		loc.T("overview_bedrock", bedrockAddress),
		loc.T("overview_players", status.PlayersOnline),
		loc.T("overview_memory", status.MemoryMaximumGB, status.TotalMemoryGB, loc.T(memoryModeKey)),
		loc.T("overview_usage"),
		loc.T("overview_uptime", formatDuration(status.Uptime)),
	}, "\n"))
	t.links.Update(status, t.u.installedVersion)
}
