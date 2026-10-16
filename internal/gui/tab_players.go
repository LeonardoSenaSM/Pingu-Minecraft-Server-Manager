package gui

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/manager"
)

const tempBanDuration = 3 * 24 * time.Hour

// playersTab: jogadores online, conhecidos e banidos.
type playersTab struct {
	u       *UI
	root    fyne.CanvasObject
	online  *fyne.Container
	offline *fyne.Container
	bans    *fyne.Container
}

func newPlayersTab(u *UI) *playersTab {
	t := &playersTab{u: u, online: container.NewVBox(), offline: container.NewVBox(), bans: container.NewVBox()}
	onlineCard := u.card("online_players", "quick_actions", container.NewVScroll(t.online))
	offlineCard := u.card("offline_players", "known_players", container.NewVScroll(t.offline))
	bansCard := u.card("banned_players", "ban_details", container.NewVScroll(t.bans))
	split := container.NewVSplit(container.NewHSplit(onlineCard, offlineCard), bansCard)
	split.Offset = 0.62
	t.root = split
	return t
}

// refresh lê os dados fora da UI e redesenha as listas.
func (t *playersTab) refresh() {
	m := t.u.manager
	online, offline, bans := m.OnlinePlayers(), m.OfflinePlayers(), m.BannedPlayers()
	t.u.do(func() {
		t.fill(t.online, online, "no_online", true)
		t.fill(t.offline, offline, "no_offline", false)
		t.fillBans(bans)
	})
}

func (t *playersTab) fill(box *fyne.Container, players []string, emptyKey string, online bool) {
	box.RemoveAll()
	if len(players) == 0 {
		box.Add(widget.NewLabel(t.u.loc.T(emptyKey)))
	}
	for _, player := range players {
		box.Add(t.playerRow(player, online))
		box.Add(widget.NewSeparator())
	}
	box.Refresh()
}

func (t *playersTab) playerRow(player string, online bool) fyne.CanvasObject {
	u, loc := t.u, t.u.loc
	name := widget.NewLabelWithStyle(player, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	op := widget.NewButton(loc.T("make_op"), func() {
		command := "op " + player
		u.confirmSensitive(command, func() { u.sendCommand("promote_failed", command) })
	})
	ban := widget.NewButton(loc.T("tempban"), func() {
		u.confirmSensitive("ban "+player, func() {
			u.runSimple("ban_failed", func(context.Context) error { return u.manager.TempBan(player, tempBanDuration) })
		})
	})
	ban.Importance = widget.DangerImportance
	actions := container.NewHBox(op)
	if online {
		actions.Add(widget.NewButton(loc.T("kick"), func() {
			u.sendCommand("kick_failed", "kick "+player+" Removed by administrator")
		}))
	}
	actions.Add(ban)
	return container.NewBorder(nil, nil, name, nil, actions)
}

func (t *playersTab) fillBans(bans []manager.BanEntry) {
	u, loc := t.u, t.u.loc
	t.bans.RemoveAll()
	if len(bans) == 0 {
		t.bans.Add(widget.NewLabel(loc.T("no_banned")))
	}
	for _, item := range bans {
		entry := item
		identity := entry.Name
		if identity == "" {
			identity = entry.IP
		}
		details := ""
		if !entry.CreatedAt.IsZero() {
			details = loc.T("banned_at", entry.CreatedAt.Format("02/01/2006 15:04"))
		}
		if !entry.ExpiresAt.IsZero() {
			details += " • " + loc.T("expires_at", entry.ExpiresAt.Format("02/01/2006 15:04"))
		}
		label := widget.NewLabel(strings.TrimSpace(identity + "\n" + details))
		pardon := widget.NewButton(loc.T("pardon"), func() {
			u.runSimple("pardon_failed", func(context.Context) error { return u.manager.Unban(entry) })
		})
		t.bans.Add(container.NewBorder(nil, nil, nil, pardon, label))
		t.bans.Add(widget.NewSeparator())
	}
	t.bans.Refresh()
}
