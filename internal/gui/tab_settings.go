package gui

import (
	"context"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/i18n"
	"mineserver/internal/manager"
)

// settingsTab: nome, max-players, memória, idioma e explorador de arquivos.
type settingsTab struct {
	u    *UI
	root fyne.CanvasObject

	serverName  *widget.Entry
	maxPlayers  *widget.Entry
	memoryMode  *widget.Select
	memoryLimit *widget.Entry
	detected    *widget.Label
	language    *widget.Select
	files       *fyne.Container
	preview     *widget.Entry

	// thread da UI
	plan             manager.MemoryPlan
	selectedMode     string
	changingMode     bool
	changingLanguage bool
}

func newSettingsTab(u *UI) *settingsTab {
	m := u.manager
	settings := m.Settings()
	t := &settingsTab{u: u, plan: m.MemoryPlan(), selectedMode: normalizeMemoryMode(settings.MemoryMode)}

	t.serverName = u.entry("server_name_placeholder")
	t.serverName.SetText(settings.ServerName)
	t.maxPlayers = u.entry("max_players_placeholder")
	t.maxPlayers.SetText(strconv.Itoa(settings.MaxPlayers))
	t.memoryLimit = widget.NewEntry()
	t.memoryLimit.SetText(strconv.Itoa(t.limitOrRecommended(settings.MemoryLimitGB)))
	t.detected = widget.NewLabel("")
	t.detected.Wrapping = fyne.TextWrapWord
	u.onTranslate(t.renderDetected)
	t.memoryMode = widget.NewSelect(nil, t.onMemoryMode)
	u.onTranslate(t.renderMemoryMode)
	t.language = widget.NewSelect(nil, t.onLanguage)
	u.onTranslate(t.renderLanguage)

	save := u.button("save_settings", theme.DocumentSaveIcon(), t.save)
	settingsCard := u.card("settings_title", "settings_subtitle", container.NewVBox(
		u.label("server_name"), t.serverName, u.label("max_players"), t.maxPlayers, widget.NewSeparator(),
		u.label("memory_mode"), t.memoryMode, u.label("memory_limit"), t.memoryLimit, t.detected,
		u.label("memory_help"), u.label("memory_restart"),
		widget.NewSeparator(), u.label("language"), t.language, save,
	))

	t.files = container.NewVBox()
	t.preview = widget.NewMultiLineEntry()
	t.preview.Disable()
	t.preview.SetPlaceHolder(u.loc.T("file_placeholder"))
	u.onTranslate(func() { t.preview.SetPlaceHolder(u.loc.T("file_placeholder")) })
	openButton := u.button("open_folder", theme.FolderOpenIcon(), func() {
		u.runSimple("open_failed", func(context.Context) error { return openFolder(m.ServerDir()) })
	})
	refreshButton := u.button("refresh_list", theme.ViewRefreshIcon(), func() { go t.refreshFiles() })
	fileSplit := container.NewHSplit(container.NewVScroll(t.files), t.preview)
	fileSplit.Offset = 0.42
	filesCard := u.card("files_title", "files_subtitle", container.NewBorder(
		container.NewHBox(openButton, refreshButton, layout.NewSpacer()), nil, nil, nil, fileSplit))

	split := container.NewVSplit(container.NewVScroll(settingsCard), filesCard)
	split.Offset = 0.36
	t.root = split
	return t
}

func normalizeMemoryMode(mode string) string {
	if mode == manager.MemoryModeManual {
		return manager.MemoryModeManual
	}
	return manager.MemoryModeAutomatic
}

func (t *settingsTab) limitOrRecommended(limit int) int {
	if limit < 1 {
		return t.plan.RecommendedGB
	}
	return limit
}

func (t *settingsTab) renderDetected() {
	t.detected.SetText(t.u.loc.T("memory_detected", t.plan.TotalGB, t.plan.RecommendedGB))
}

func (t *settingsTab) renderMemoryMode() {
	loc := t.u.loc
	t.changingMode = true
	t.memoryMode.Options = []string{loc.T("memory_auto"), loc.T("memory_manual")}
	if t.selectedMode == manager.MemoryModeManual {
		t.memoryMode.SetSelected(loc.T("memory_manual"))
		t.memoryLimit.Enable()
	} else {
		t.memoryMode.SetSelected(loc.T("memory_auto"))
		t.memoryLimit.Disable()
	}
	t.memoryMode.Refresh()
	t.changingMode = false
}

func (t *settingsTab) onMemoryMode(display string) {
	if t.changingMode {
		return
	}
	if display == t.u.loc.T("memory_manual") {
		t.selectedMode = manager.MemoryModeManual
		t.memoryLimit.Enable()
		return
	}
	t.selectedMode = manager.MemoryModeAutomatic
	t.memoryLimit.SetText(strconv.Itoa(t.plan.RecommendedGB))
	t.memoryLimit.Disable()
}

func (t *settingsTab) renderLanguage() {
	loc := t.u.loc
	t.changingLanguage = true
	t.language.Options = []string{loc.T("english"), loc.T("portuguese")}
	if loc.Language() == i18n.English {
		t.language.SetSelected(loc.T("english"))
	} else {
		t.language.SetSelected(loc.T("portuguese"))
	}
	t.changingLanguage = false
}

func (t *settingsTab) onLanguage(display string) {
	if t.changingLanguage {
		return
	}
	language := i18n.English
	if display == t.u.loc.T("portuguese") {
		language = i18n.Portuguese
	}
	t.u.loc.SetLanguage(language)
	t.u.manager.SetLanguage(language)
	t.u.retranslate()
}

func (t *settingsTab) save() {
	u := t.u
	serverName := strings.TrimSpace(t.serverName.Text)
	maxPlayers, err := strconv.Atoi(strings.TrimSpace(t.maxPlayers.Text))
	if serverName == "" || err != nil || maxPlayers < 1 || maxPlayers > 1000 {
		u.showInfo("settings_failed", "invalid_settings")
		return
	}
	memoryMode, memoryLimitGB := t.selectedMode, 0
	if memoryMode == manager.MemoryModeManual {
		memoryLimitGB, err = strconv.Atoi(strings.TrimSpace(t.memoryLimit.Text))
		if err != nil {
			u.showInfo("settings_failed", "invalid_settings")
			return
		}
	}
	language := u.loc.Language()
	go func() {
		if err := u.manager.SaveSettingsWithMemory(language, serverName, maxPlayers, memoryMode, memoryLimitGB); err != nil {
			u.showError("settings_failed", err)
			return
		}
		u.showInfo("settings_saved", "settings_saved_message")
	}()
}

// apply reflete configurações salvas. Thread da UI.
func (t *settingsTab) apply(saved manager.Settings) {
	t.serverName.SetText(saved.ServerName)
	t.maxPlayers.SetText(strconv.Itoa(saved.MaxPlayers))
	t.selectedMode = normalizeMemoryMode(saved.MemoryMode)
	t.plan = t.u.manager.MemoryPlan()
	t.memoryLimit.SetText(strconv.Itoa(t.limitOrRecommended(saved.MemoryLimitGB)))
	t.renderDetected()
	t.renderMemoryMode()
}

// refreshFiles lista a pasta do servidor fora da UI.
func (t *settingsTab) refreshFiles() {
	u := t.u
	entries, err := u.manager.ListServerFiles()
	if err != nil {
		u.showError("list_failed", err)
		return
	}
	t.u.do(func() {
		t.files.RemoveAll()
		if len(entries) == 0 {
			t.files.Add(widget.NewLabel(u.loc.T("empty_folder")))
		}
		for _, entry := range entries {
			item := entry
			button := widget.NewButton(item.Display, func() { t.showFile(item) })
			button.Alignment = widget.ButtonAlignLeading
			t.files.Add(button)
		}
		t.files.Refresh()
	})
}

func (t *settingsTab) showFile(item manager.FileEntry) {
	u := t.u
	if item.IsDir {
		t.preview.SetText(u.loc.T("folder", item.Relative))
		return
	}
	go func() {
		content, err := u.manager.ReadServerFile(item.Relative)
		t.u.do(func() {
			if err != nil {
				t.preview.SetText(u.loc.T("preview_error", err.Error()))
				return
			}
			t.preview.SetText(content)
		})
	}()
}
