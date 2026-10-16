package gui

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/commands"
	"mineserver/internal/manager"
)

const (
	// consoleLines limita o que o console redesenha. O histórico completo
	// continua no Manager; o Entry do Fyne fica lento com milhares de linhas.
	consoleLines         = 1000
	consoleRefreshPeriod = 250 * time.Millisecond
)

// consoleTab: log do servidor, envio de comandos e catálogo pesquisável.
type consoleTab struct {
	u       *UI
	root    fyne.CanvasObject
	output  *widget.Entry
	command *widget.Entry
	search  *widget.Entry
	catalog *fyne.Container
	signal  chan struct{}
}

func newConsoleTab(u *UI) *consoleTab {
	t := &consoleTab{u: u, signal: make(chan struct{}, 1)}
	t.output = widget.NewMultiLineEntry()
	t.output.Disable()
	t.output.Wrapping = fyne.TextWrapOff
	t.output.SetPlaceHolder(u.loc.T("logs_placeholder"))
	u.onTranslate(func() { t.output.SetPlaceHolder(u.loc.T("logs_placeholder")) })

	t.command = u.entry("command_placeholder")
	t.command.OnSubmitted = func(string) { t.submit() }
	send := u.button("send", theme.MailSendIcon(), t.submit)

	t.search = u.entry("search_commands")
	t.search.OnChanged = func(string) { t.renderCatalog() }
	t.catalog = container.NewVBox()
	u.onTranslate(t.renderCatalog)
	catalogCard := u.card("command_catalog", "command_catalog_subtitle",
		container.NewBorder(t.search, nil, nil, nil, container.NewVScroll(t.catalog)))

	split := container.NewHSplit(t.output, catalogCard)
	split.Offset = 0.70
	t.root = container.NewBorder(nil, container.NewBorder(nil, nil, nil, send, t.command), nil, nil, split)
	return t
}

// notify é chamado pelo Manager a cada linha de log, de qualquer goroutine.
// Só marca que há novidade; o redesenho acontece no ritmo de startPump.
func (t *consoleTab) notify(manager.LogEntry) {
	select {
	case t.signal <- struct{}{}:
	default:
	}
}

func (t *consoleTab) startPump(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(consoleRefreshPeriod)
		defer ticker.Stop()
		dirty := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.signal:
				dirty = true
			case <-ticker.C:
				if !dirty {
					continue
				}
				dirty = false
				text := t.u.manager.LogTail(consoleLines)
				t.u.do(func() {
					t.output.SetText(text)
					t.output.CursorRow = strings.Count(text, "\n")
					t.output.CursorColumn = 0
					t.output.Refresh()
				})
			}
		}
	}()
}

func (t *consoleTab) submit() {
	command := strings.TrimSpace(t.command.Text)
	if command == "" {
		return
	}
	sendNow := func() {
		t.command.SetText("")
		t.u.sendCommand("command_failed", command)
	}
	if commands.RiskForInput(command) >= commands.RiskSensitive {
		t.u.confirmSensitive(command, sendNow)
		return
	}
	sendNow()
}

func (t *consoleTab) renderCatalog() {
	loc := t.u.loc
	t.catalog.RemoveAll()
	for _, item := range commands.Filter(t.search.Text, func(key string) string { return loc.T(key) }) {
		command := item
		button := widget.NewButton(command.Syntax, func() {
			t.command.SetText(commands.SelectionText(command))
			t.u.window.Canvas().Focus(t.command)
		})
		button.Alignment = widget.ButtonAlignLeading
		if command.Risk == commands.RiskDangerous {
			button.Importance = widget.DangerImportance
		}
		description := widget.NewLabel(loc.T(command.DescriptionKey) + " • " + loc.T(command.CategoryKey))
		description.TextStyle = fyne.TextStyle{Italic: true}
		description.Wrapping = fyne.TextWrapWord
		t.catalog.Add(container.NewVBox(button, description, widget.NewSeparator()))
	}
	t.catalog.Refresh()
}
