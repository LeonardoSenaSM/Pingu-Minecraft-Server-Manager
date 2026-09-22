package gui

import (
	"fmt"
	"image/color"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/i18n"
	"mineserver/internal/manager"
)

var (
	linkSuccessColor = color.NRGBA{R: 52, G: 211, B: 153, A: 255}
	linkOfflineColor = color.NRGBA{R: 148, G: 163, B: 184, A: 255}
)

// LinkControls is the Home-tab connection module. Update and Retranslate must
// be called on Fyne's UI goroutine; callers receiving background events should
// wrap those calls in fyne.Do.
type LinkControls struct {
	Root fyne.CanvasObject

	window   fyne.Window
	loc      *i18n.Localizer
	shutdown func() error

	title       *widget.Label
	subtitle    *widget.Label
	javaCard    *widget.Card
	bedrockCard *widget.Card
	javaVersion *widget.Label
	bedVersion  *widget.Label
	javaAddress *widget.Label
	bedAddress  *widget.Label
	javaBadge   *canvas.Text
	bedBadge    *canvas.Text
	javaCopy    *widget.Button
	bedCopy     *widget.Button
	javaToast   *canvas.Text
	bedToast    *canvas.Text
	shutdownBtn *widget.Button

	javaLink string
	bedLink  string
	version  string
	status   manager.Status
	toastID  atomic.Uint64
}

// BuildLinkControls creates a self-contained, responsive connection panel.
// shutdown is deliberately synchronous: the component always invokes it from
// a background goroutine so closing sockets can never block Fyne rendering.
func BuildLinkControls(window fyne.Window, loc *i18n.Localizer, shutdown func() error) *LinkControls {
	controls := &LinkControls{window: window, loc: loc, shutdown: shutdown}
	controls.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	controls.subtitle = widget.NewLabel("")
	controls.subtitle.Wrapping = fyne.TextWrapWord
	controls.javaVersion = widget.NewLabel("")
	controls.javaVersion.Wrapping = fyne.TextWrapWord
	controls.bedVersion = widget.NewLabel("")
	controls.bedVersion.Wrapping = fyne.TextWrapWord
	controls.javaAddress = widget.NewLabel("")
	controls.javaAddress.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	controls.javaAddress.Wrapping = fyne.TextWrapWord
	controls.bedAddress = widget.NewLabel("")
	controls.bedAddress.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	controls.bedAddress.Wrapping = fyne.TextWrapWord
	controls.javaBadge = canvas.NewText("", linkOfflineColor)
	controls.javaBadge.TextStyle = fyne.TextStyle{Bold: true}
	controls.bedBadge = canvas.NewText("", linkOfflineColor)
	controls.bedBadge.TextStyle = fyne.TextStyle{Bold: true}
	controls.javaToast = canvas.NewText("", linkSuccessColor)
	controls.javaToast.TextStyle = fyne.TextStyle{Bold: true}
	controls.javaToast.Hide()
	controls.bedToast = canvas.NewText("", linkSuccessColor)
	controls.bedToast.TextStyle = fyne.TextStyle{Bold: true}
	controls.bedToast.Hide()

	controls.javaCopy = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		controls.copyLink(controls.javaLink, controls.javaToast)
	})
	controls.bedCopy = widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		controls.copyLink(controls.bedLink, controls.bedToast)
	})

	javaBody := container.NewVBox(
		controls.javaVersion,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, controls.javaBadge, nil, controls.javaAddress),
		container.NewBorder(nil, nil, controls.javaToast, nil, layout.NewSpacer()),
		controls.javaCopy,
	)
	bedrockBody := container.NewVBox(
		controls.bedVersion,
		widget.NewSeparator(),
		container.NewBorder(nil, nil, controls.bedBadge, nil, controls.bedAddress),
		container.NewBorder(nil, nil, controls.bedToast, nil, layout.NewSpacer()),
		controls.bedCopy,
	)
	controls.javaCard = widget.NewCard("", "", container.NewPadded(javaBody))
	controls.bedrockCard = widget.NewCard("", "", container.NewPadded(bedrockBody))
	controls.shutdownBtn = widget.NewButtonWithIcon("", theme.MediaStopIcon(), controls.confirmShutdown)
	controls.shutdownBtn.Importance = widget.DangerImportance

	header := container.NewVBox(controls.title, controls.subtitle, widget.NewSeparator())
	cards := container.NewGridWithColumns(2, controls.javaCard, controls.bedrockCard)
	footer := container.NewBorder(nil, nil, layout.NewSpacer(), controls.shutdownBtn)
	controls.Root = container.NewPadded(container.NewVBox(header, cards, footer))
	controls.Retranslate()
	controls.Update(manager.Status{}, "")
	return controls
}

func (c *LinkControls) Update(status manager.Status, paperVersion string) {
	c.status = status
	if strings.TrimSpace(paperVersion) != "" {
		c.version = strings.TrimSpace(paperVersion)
	}
	address := c.loc.T("link_waiting")
	if len(status.LocalIPs) > 0 {
		address = status.LocalIPs[0]
	}
	c.javaLink = buildLink(address, status.JavaPort, status.JavaListening)
	c.bedLink = buildLink(address, status.BedrockPort, status.BedrockListening)
	c.javaAddress.SetText(displayLink(c.javaLink, c.loc.T("link_waiting")))
	c.bedAddress.SetText(displayLink(c.bedLink, c.loc.T("link_waiting")))
	c.setBadge(c.javaBadge, status.JavaListening)
	c.setBadge(c.bedBadge, status.BedrockListening)
	setButtonEnabled(c.javaCopy, status.JavaListening && c.javaLink != "")
	setButtonEnabled(c.bedCopy, status.BedrockListening && c.bedLink != "")
	setButtonEnabled(c.shutdownBtn, status.JavaListening || status.BedrockListening)
	c.updateVersions()
}

func (c *LinkControls) Retranslate() {
	c.title.SetText(c.loc.T("links_title"))
	c.subtitle.SetText(c.loc.T("links_subtitle"))
	c.javaCard.Title = c.loc.T("java_edition")
	c.javaCard.Subtitle = c.loc.T("java_protocol")
	c.javaCard.Refresh()
	c.bedrockCard.Title = c.loc.T("bedrock_crossplay")
	c.bedrockCard.Subtitle = c.loc.T("bedrock_protocol")
	c.bedrockCard.Refresh()
	c.javaCopy.SetText(c.loc.T("copy_link"))
	c.bedCopy.SetText(c.loc.T("copy_link"))
	c.shutdownBtn.SetText(c.loc.T("shutdown_links"))
	c.javaToast.Text = c.loc.T("link_copied")
	c.javaToast.Refresh()
	c.bedToast.Text = c.loc.T("link_copied")
	c.bedToast.Refresh()
	c.Update(c.status, c.version)
}

func (c *LinkControls) updateVersions() {
	version := strings.TrimSpace(c.version)
	if version == "" || strings.Contains(strings.ToLower(version), "identificada") {
		version = c.loc.T("version_not_installed")
	} else if !strings.HasPrefix(strings.ToLower(version), "v") {
		version = "v" + version
	}
	c.javaVersion.SetText(c.loc.T("supported_version", version))
	c.bedVersion.SetText(c.loc.T("supported_version", c.loc.T("bedrock_supported_range")))
}

func (c *LinkControls) setBadge(badge *canvas.Text, active bool) {
	if active {
		badge.Text = c.loc.T("link_on")
		badge.Color = linkSuccessColor
	} else {
		badge.Text = c.loc.T("link_off")
		badge.Color = linkOfflineColor
	}
	badge.Refresh()
}

func (c *LinkControls) copyLink(link string, toast *canvas.Text) {
	if link == "" {
		return
	}
	c.window.Clipboard().SetContent(link)
	toast.Show()
	toast.Refresh()
	id := c.toastID.Add(1)
	go func() {
		timer := time.NewTimer(1600 * time.Millisecond)
		defer timer.Stop()
		<-timer.C
		fyne.Do(func() {
			if c.toastID.Load() == id {
				toast.Hide()
				toast.Refresh()
			}
		})
	}()
}

func (c *LinkControls) confirmShutdown() {
	confirmation := dialog.NewConfirm(
		c.loc.T("shutdown_confirm_title"),
		c.loc.T("shutdown_confirm_message"),
		func(confirmed bool) {
			if !confirmed || c.shutdown == nil {
				return
			}
			c.shutdownBtn.Disable()
			go func() {
				err := c.shutdown()
				fyne.Do(func() {
					if err != nil {
						dialog.ShowError(fmt.Errorf("%s: %w", c.loc.T("shutdown_failed"), err), c.window)
						c.shutdownBtn.Enable()
						return
					}
					dialog.ShowInformation(c.loc.T("shutdown_complete"), c.loc.T("shutdown_complete_message"), c.window)
				})
			}()
		},
		c.window,
	)
	confirmation.SetConfirmText(c.loc.T("shutdown_links"))
	confirmation.SetDismissText(c.loc.T("cancel"))
	confirmation.Show()
}

func buildLink(address string, port int, active bool) string {
	if !active || port <= 0 || strings.TrimSpace(address) == "" {
		return ""
	}
	return net.JoinHostPort(address, strconv.Itoa(port))
}

func displayLink(link, fallback string) string {
	if link == "" {
		return fallback
	}
	return link
}

func setButtonEnabled(button *widget.Button, enabled bool) {
	if enabled {
		button.Enable()
	} else {
		button.Disable()
	}
}
