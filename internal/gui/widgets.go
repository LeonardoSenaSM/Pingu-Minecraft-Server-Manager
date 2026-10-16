package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/manager"
)

// Fábricas de widgets que já se registram para a troca de idioma. Assim cada
// aba não precisa manter uma lista manual de textos a retraduzir.

func (u *UI) button(key string, icon fyne.Resource, tapped func()) *widget.Button {
	b := widget.NewButtonWithIcon(u.loc.T(key), icon, tapped)
	u.onTranslate(func() { b.SetText(u.loc.T(key)) })
	return b
}

func (u *UI) label(key string) *widget.Label {
	l := widget.NewLabel(u.loc.T(key))
	l.Wrapping = fyne.TextWrapWord
	u.onTranslate(func() { l.SetText(u.loc.T(key)) })
	return l
}

func (u *UI) card(titleKey, subtitleKey string, content fyne.CanvasObject) *widget.Card {
	c := widget.NewCard(u.loc.T(titleKey), u.loc.T(subtitleKey), content)
	u.onTranslate(func() {
		c.Title, c.Subtitle = u.loc.T(titleKey), u.loc.T(subtitleKey)
		c.Refresh()
	})
	return c
}

func (u *UI) entry(placeholderKey string) *widget.Entry {
	e := widget.NewEntry()
	e.SetPlaceHolder(u.loc.T(placeholderKey))
	u.onTranslate(func() { e.SetPlaceHolder(u.loc.T(placeholderKey)) })
	return e
}

// commandButton envia um comando fixo ao servidor.
func (u *UI) commandButton(key, command string) *widget.Button {
	return u.button(key, nil, func() { u.sendCommand("command_failed", command) })
}

// progressPanel mostra o andamento de downloads e backups.
type progressPanel struct {
	root   fyne.CanvasObject
	label  *widget.Label
	bar    *widget.ProgressBar
	cancel *widget.Button
	u      *UI
	idle   bool
	task   string
}

func newProgressPanel(u *UI) *progressPanel {
	p := &progressPanel{u: u, idle: true}
	p.label = widget.NewLabel(u.loc.T("progress_idle"))
	p.bar = widget.NewProgressBar()
	p.bar.Min, p.bar.Max = 0, 1
	p.cancel = u.button("cancel", nil, u.cancelOperation)
	p.cancel.Disable()
	p.root = container.NewBorder(nil, nil, p.label, p.cancel, p.bar)
	u.onTranslate(p.renderLabel)
	return p
}

func (p *progressPanel) renderLabel() {
	if p.idle {
		p.label.SetText(p.u.loc.T("progress_idle"))
	} else {
		p.label.SetText(p.u.loc.T("progress_working", p.task))
	}
}

// update é chamado na thread da UI.
func (p *progressPanel) update(progress manager.Progress) {
	if progress.Done {
		p.idle, p.task = true, ""
		p.bar.SetValue(0)
		p.cancel.Disable()
		p.renderLabel()
		return
	}
	p.idle, p.task = false, progress.Task
	p.cancel.Enable()
	p.renderLabel()
	value := 0.0
	if progress.Total > 0 {
		value = float64(progress.Downloaded) / float64(progress.Total)
	} else if progress.TotalTasks > 0 {
		value = float64(progress.Completed) / float64(progress.TotalTasks)
	}
	p.bar.SetValue(value)
}
