package gui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mineserver/internal/api"
	"mineserver/internal/commands"
	"mineserver/internal/i18n"
	"mineserver/internal/manager"
)

type UI struct {
	manager *manager.Manager
	app     fyne.App
	window  fyne.Window
	loc     *i18n.Localizer

	opMu     sync.Mutex
	opCancel context.CancelFunc
	busy     bool
}

func New(app fyne.App, window fyne.Window, service *manager.Manager) *UI {
	settings := service.Settings()
	return &UI{manager: service, app: app, window: window, loc: i18n.New(settings.Language)}
}

func (u *UI) Build() fyne.CanvasObject {
	m := u.manager
	loc := u.loc
	serverStatus := widget.NewLabel(loc.T(manager.StateStopped))
	serverStatus.TextStyle = fyne.TextStyle{Bold: true}
	networkStatus := widget.NewLabel(loc.T("inactive"))
	networkStatus.TextStyle = fyne.TextStyle{Bold: true}
	overviewText := widget.NewLabel("")
	overviewText.Wrapping = fyne.TextWrapWord
	installedValue := ""
	installedLoaded := false
	lastStatus := manager.Status{}
	var linkControls *LinkControls

	progressLabel := widget.NewLabel(loc.T("progress_idle"))
	progressBar := widget.NewProgressBar()
	progressBar.Min = 0
	progressBar.Max = 1
	cancelProgressButton := widget.NewButton(loc.T("cancel"), u.cancelOperation)
	cancelProgressButton.Disable()
	progressPanel := container.NewBorder(nil, nil, progressLabel, cancelProgressButton, progressBar)

	console := widget.NewMultiLineEntry()
	console.Disable()
	console.Wrapping = fyne.TextWrapOff
	console.SetPlaceHolder(loc.T("logs_placeholder"))

	onlineBox := container.NewVBox()
	offlineBox := container.NewVBox()
	bansBox := container.NewVBox()
	filesBox := container.NewVBox()
	filePreview := widget.NewMultiLineEntry()
	filePreview.Disable()
	filePreview.SetPlaceHolder(loc.T("file_placeholder"))

	showError := func(titleKey string, err error) {
		fyne.Do(func() { dialog.ShowError(fmt.Errorf("%s: %w", loc.T(titleKey), err), u.window) })
	}
	showInfo := func(titleKey, messageKey string, args ...any) {
		fyne.Do(func() { dialog.ShowInformation(loc.T(titleKey), loc.T(messageKey, args...), u.window) })
	}
	confirmSensitive := func(command string, confirmed func()) {
		dialog.ShowConfirm(loc.T("confirm_command_title"), loc.T("confirm_command_message", command), func(ok bool) {
			if ok {
				confirmed()
			}
		}, u.window)
	}

	refreshStatus := func(status manager.Status) {
		fyne.Do(func() {
			lastStatus = status
			serverStatus.SetText(loc.T(status.ServerState))
			if status.NetworkActive {
				networkStatus.SetText(loc.T("listening"))
			} else {
				networkStatus.SetText(loc.T("inactive"))
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
			overviewText.SetText(strings.Join([]string{
				loc.T("overview_server", status.ServerName),
				loc.T("overview_version", minecraftVersion),
				loc.T("overview_java", javaAddress),
				loc.T("overview_bedrock", bedrockAddress),
				loc.T("overview_players", status.PlayersOnline),
				loc.T("overview_memory", status.MemoryMaximumGB, status.TotalMemoryGB, loc.T(memoryModeKey)),
				loc.T("overview_usage"),
				loc.T("overview_uptime", formatDuration(status.Uptime)),
			}, "\n"))
			if linkControls != nil {
				linkControls.Update(status, installedValue)
			}
		})
	}
	consoleSignal := make(chan struct{}, 1)
	refreshConsole := func(manager.LogEntry) {
		select {
		case consoleSignal <- struct{}{}:
		default:
		}
	}
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		dirty := false
		for {
			select {
			case <-m.Context().Done():
				return
			case <-consoleSignal:
				dirty = true
			case <-ticker.C:
				if !dirty {
					continue
				}
				dirty = false
				text := m.LogText()
				fyne.Do(func() {
					console.SetText(text)
					console.CursorRow = strings.Count(text, "\n")
					console.CursorColumn = 0
					console.Refresh()
				})
			}
		}
	}()

	var refreshPlayers func()
	refreshPlayers = func() {
		online := m.OnlinePlayers()
		offline := m.OfflinePlayers()
		bans := m.BannedPlayers()
		fyne.Do(func() {
			playerRow := func(player string, onlineNow bool) fyne.CanvasObject {
				name := widget.NewLabel(player)
				name.TextStyle = fyne.TextStyle{Bold: true}
				op := widget.NewButton(loc.T("make_op"), func() {
					command := "op " + player
					confirmSensitive(command, func() {
						u.runSimple("promote_failed", func(context.Context) error { return m.SendCommand(command) }, showError)
					})
				})
				ban := widget.NewButton(loc.T("tempban"), func() {
					confirmSensitive("ban "+player, func() {
						u.runSimple("ban_failed", func(context.Context) error { return m.TempBan(player, 3*24*time.Hour) }, showError)
					})
				})
				ban.Importance = widget.DangerImportance
				actions := container.NewHBox(op)
				if onlineNow {
					kick := widget.NewButton(loc.T("kick"), func() {
						u.runSimple("kick_failed", func(context.Context) error { return m.SendCommand("kick " + player + " Removed by administrator") }, showError)
					})
					actions.Add(kick)
				}
				actions.Add(ban)
				return container.NewBorder(nil, nil, name, nil, actions)
			}
			onlineBox.RemoveAll()
			if len(online) == 0 {
				onlineBox.Add(widget.NewLabel(loc.T("no_online")))
			} else {
				for _, player := range online {
					onlineBox.Add(playerRow(player, true))
					onlineBox.Add(widget.NewSeparator())
				}
			}
			offlineBox.RemoveAll()
			if len(offline) == 0 {
				offlineBox.Add(widget.NewLabel(loc.T("no_offline")))
			} else {
				for _, player := range offline {
					offlineBox.Add(playerRow(player, false))
					offlineBox.Add(widget.NewSeparator())
				}
			}
			bansBox.RemoveAll()
			if len(bans) == 0 {
				bansBox.Add(widget.NewLabel(loc.T("no_banned")))
			} else {
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
						u.runSimple("pardon_failed", func(context.Context) error { return m.Unban(entry) }, showError)
					})
					bansBox.Add(container.NewBorder(nil, nil, nil, pardon, label))
					bansBox.Add(widget.NewSeparator())
				}
			}
			onlineBox.Refresh()
			offlineBox.Refresh()
			bansBox.Refresh()
		})
	}

	var refreshFiles func()
	refreshFiles = func() {
		entries, err := m.ListServerFiles()
		if err != nil {
			showError("list_failed", err)
			return
		}
		fyne.Do(func() {
			filesBox.RemoveAll()
			if len(entries) == 0 {
				filesBox.Add(widget.NewLabel(loc.T("empty_folder")))
			}
			for _, entry := range entries {
				item := entry
				button := widget.NewButton(item.Display, func() {
					if item.IsDir {
						filePreview.SetText(loc.T("folder", item.Relative))
						return
					}
					go func() {
						content, readErr := m.ReadServerFile(item.Relative)
						fyne.Do(func() {
							if readErr != nil {
								filePreview.SetText(loc.T("preview_error", readErr.Error()))
							} else {
								filePreview.SetText(content)
							}
						})
					}()
				})
				button.Alignment = widget.ButtonAlignLeading
				filesBox.Add(button)
			}
			filesBox.Refresh()
		})
	}

	jvmCard := widget.NewCard(loc.T("jvm_title"), loc.T("jvm_subtitle"), serverStatus)
	networkCard := widget.NewCard(loc.T("network_title"), loc.T("network_subtitle"), networkStatus)
	overviewCard := widget.NewCard(loc.T("overview_title"), loc.T("overview_subtitle"), overviewText)
	linkControls = BuildLinkControls(u.window, loc, m.ShutdownLinks)
	playButton := widget.NewButtonWithIcon(loc.T("play"), theme.MediaPlayIcon(), func() {
		u.runOperation(func(ctx context.Context) error { return m.StartAll(ctx) }, func(err error) {
			if err != nil {
				showError("start_failed", err)
			}
		}, cancelProgressButton)
	})
	playButton.Importance = widget.HighImportance
	stopButton := widget.NewButtonWithIcon(loc.T("stop_server"), theme.MediaStopIcon(), func() {
		confirmSensitive("stop", func() {
			u.runSimple("stop_failed", func(context.Context) error { return m.StopServer(false) }, showError)
		})
	})
	stopButton.Importance = widget.DangerImportance
	restartButton := widget.NewButtonWithIcon(loc.T("restart_server"), theme.ViewRefreshIcon(), func() {
		u.runSimple("restart_failed", m.RestartServer, showError)
	})
	updateButton := widget.NewButtonWithIcon(loc.T("update_dependencies"), theme.DownloadIcon(), func() {
		u.runOperation(func(ctx context.Context) error { return m.UpdateDependencies(ctx) }, func(err error) {
			if err != nil {
				showError("update_failed", err)
				return
			}
			showInfo("updated_title", "updated_message")
			go refreshFiles()
		}, cancelProgressButton)
	})
	homeHelp := widget.NewLabel(loc.T("quick_guide"))
	homeHelp.Wrapping = fyne.TextWrapWord
	home := container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle("Pingu", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		playButton, overviewCard, linkControls.Root, container.NewGridWithColumns(2, jvmCard, networkCard),
		container.NewGridWithColumns(3, stopButton, restartButton, updateButton), progressPanel, homeHelp,
	))

	commandEntry := widget.NewEntry()
	commandEntry.SetPlaceHolder(loc.T("command_placeholder"))
	var sendCommand func()
	sendNow := func(command string) {
		commandEntry.SetText("")
		u.runSimple("command_failed", func(context.Context) error { return m.SendCommand(command) }, showError)
	}
	sendCommand = func() {
		command := strings.TrimSpace(commandEntry.Text)
		if command == "" {
			return
		}
		if commands.RiskForInput(command) >= commands.RiskSensitive {
			dialog.ShowConfirm(loc.T("confirm_command_title"), loc.T("confirm_command_message", command), func(confirmed bool) {
				if confirmed {
					sendNow(command)
				}
			}, u.window)
			return
		}
		sendNow(command)
	}
	commandEntry.OnSubmitted = func(string) { sendCommand() }
	sendButton := widget.NewButtonWithIcon(loc.T("send"), theme.MailSendIcon(), sendCommand)
	commandSearch := widget.NewEntry()
	commandSearch.SetPlaceHolder(loc.T("search_commands"))
	commandCatalogBox := container.NewVBox()
	var refreshCommandCatalog func()
	refreshCommandCatalog = func() {
		query := commandSearch.Text
		commandCatalogBox.RemoveAll()
		for _, item := range commands.Filter(query, func(key string) string { return loc.T(key) }) {
			command := item
			button := widget.NewButton(command.Syntax, func() {
				commandEntry.SetText(commands.SelectionText(command))
				u.window.Canvas().Focus(commandEntry)
			})
			button.Alignment = widget.ButtonAlignLeading
			if command.Risk == commands.RiskDangerous {
				button.Importance = widget.DangerImportance
			}
			description := widget.NewLabel(loc.T(command.DescriptionKey) + " • " + loc.T(command.CategoryKey))
			description.TextStyle = fyne.TextStyle{Italic: true}
			description.Wrapping = fyne.TextWrapWord
			commandCatalogBox.Add(container.NewVBox(button, description, widget.NewSeparator()))
		}
		commandCatalogBox.Refresh()
	}
	commandSearch.OnChanged = func(string) { refreshCommandCatalog() }
	catalogCard := widget.NewCard(loc.T("command_catalog"), loc.T("command_catalog_subtitle"), container.NewBorder(commandSearch, nil, nil, nil, container.NewVScroll(commandCatalogBox)))
	consoleSplit := container.NewHSplit(console, catalogCard)
	consoleSplit.Offset = 0.70
	terminal := container.NewBorder(nil, container.NewBorder(nil, nil, nil, sendButton, commandEntry), nil, nil, consoleSplit)

	onlineCard := widget.NewCard(loc.T("online_players"), loc.T("quick_actions"), container.NewVScroll(onlineBox))
	offlineCard := widget.NewCard(loc.T("offline_players"), loc.T("known_players"), container.NewVScroll(offlineBox))
	bansCard := widget.NewCard(loc.T("banned_players"), loc.T("ban_details"), container.NewVScroll(bansBox))
	playerManager := container.NewVSplit(container.NewHSplit(onlineCard, offlineCard), bansCard)
	playerManager.Offset = 0.62

	availableVersions := []string{api.LatestVersion}
	versionValues := make(map[string]string)
	versionSelect := widget.NewSelect(nil, nil)
	setVersionOptions := func() {
		versionValues = make(map[string]string, len(availableVersions))
		options := make([]string, 0, len(availableVersions))
		selected := m.SelectedVersion()
		selectedDisplay := ""
		for _, version := range availableVersions {
			display := version
			if version == api.LatestVersion {
				display = loc.T("latest_stable")
			}
			versionValues[display] = version
			options = append(options, display)
			if version == selected {
				selectedDisplay = display
			}
		}
		versionSelect.Options = options
		versionSelect.Refresh()
		if selectedDisplay == "" && len(options) > 0 {
			selectedDisplay = options[0]
		}
		versionSelect.SetSelected(selectedDisplay)
	}
	versionSelect.OnChanged = func(display string) {
		version := versionValues[display]
		if version == "" {
			return
		}
		if err := m.SetSelectedVersion(version); err != nil {
			showError("settings_failed", err)
			return
		}
		go func() {
			if err := m.SaveCurrentSettings(); err != nil {
				m.Log("error", "settings", "falha ao salvar versão selecionada", err)
			}
		}()
	}
	setVersionOptions()
	installedLabel := widget.NewLabel(loc.T("installed_version", loc.T("loading")))
	installedLabel.Wrapping = fyne.TextWrapWord
	applyVersionButton := widget.NewButtonWithIcon(loc.T("apply_version"), theme.DownloadIcon(), func() {
		u.runOperation(func(ctx context.Context) error { return m.UpdatePaperVersion(ctx) }, func(err error) {
			if err != nil {
				showError("apply_failed", err)
				return
			}
			installed := m.InstalledPaperVersion()
			status := m.Status()
			fyne.Do(func() {
				installedValue = installed
				installedLoaded = true
				installedLabel.SetText(loc.T("installed_version", installed))
				linkControls.Update(status, installedValue)
			})
			showInfo("version_applied", "version_ready", installed)
		}, cancelProgressButton)
	})
	loadVersions := func() {
		versions, err := m.AvailablePaperVersions(m.Context())
		if err != nil {
			showError("versions_failed", err)
			return
		}
		fyne.Do(func() { availableVersions = versions; setVersionOptions() })
	}
	refreshVersionsButton := widget.NewButtonWithIcon(loc.T("refresh_list"), theme.ViewRefreshIcon(), func() { go loadVersions() })
	commandButton := func(labelKey, command string) *widget.Button {
		return widget.NewButton(loc.T(labelKey), func() {
			u.runSimple("command_failed", func(context.Context) error { return m.SendCommand(command) }, showError)
		})
	}
	saveWorldButton := commandButton("save_world", "save-all")
	clearButton := commandButton("clear", "weather clear")
	rainButton := commandButton("rain", "weather rain")
	thunderButton := commandButton("thunder", "weather thunder")
	dayButton := commandButton("day", "time set day")
	nightButton := commandButton("night", "time set night")
	peacefulButton := commandButton("peaceful", "difficulty peaceful")
	easyButton := commandButton("easy", "difficulty easy")
	normalButton := commandButton("normal", "difficulty normal")
	hardButton := commandButton("hard", "difficulty hard")
	playerName := widget.NewEntry()
	playerName.SetPlaceHolder(loc.T("player_name"))
	playerCommandButton := func(labelKey, command string) *widget.Button {
		return widget.NewButton(loc.T(labelKey), func() {
			name := strings.TrimSpace(playerName.Text)
			if name == "" {
				showInfo("name_required", "name_required_message")
				return
			}
			u.runSimple("command_failed", func(context.Context) error { return m.SendCommand(command + " " + name) }, showError)
		})
	}
	survivalButton := playerCommandButton("survival", "gamemode survival")
	creativeButton := playerCommandButton("creative", "gamemode creative")
	versionWarning := widget.NewLabel(loc.T("version_warning"))
	versionWarning.Wrapping = fyne.TextWrapWord
	versionCard := widget.NewCard(loc.T("minecraft_version"), loc.T("version_subtitle"), container.NewVBox(versionSelect, installedLabel, container.NewGridWithColumns(2, applyVersionButton, refreshVersionsButton), versionWarning))
	generalCard := widget.NewCard(loc.T("general"), loc.T("general_subtitle"), saveWorldButton)
	weatherCard := widget.NewCard(loc.T("weather"), loc.T("weather_subtitle"), container.NewGridWithColumns(3, clearButton, rainButton, thunderButton))
	timeCard := widget.NewCard(loc.T("time"), loc.T("time_subtitle"), container.NewGridWithColumns(2, dayButton, nightButton))
	difficultyCard := widget.NewCard(loc.T("difficulty"), loc.T("difficulty_subtitle"), container.NewGridWithColumns(4, peacefulButton, easyButton, normalButton, hardButton))
	gamemodeCard := widget.NewCard(loc.T("gamemode"), loc.T("gamemode_subtitle"), container.NewVBox(playerName, container.NewGridWithColumns(2, survivalButton, creativeButton)))
	controls := container.NewVScroll(container.NewVBox(versionCard, generalCard, weatherCard, timeCard, difficultyCard, gamemodeCard))

	settings := m.Settings()
	serverNameLabel := widget.NewLabel(loc.T("server_name"))
	serverNameEntry := widget.NewEntry()
	serverNameEntry.SetText(settings.ServerName)
	maxPlayersLabel := widget.NewLabel(loc.T("max_players"))
	maxPlayersEntry := widget.NewEntry()
	maxPlayersEntry.SetText(strconv.Itoa(settings.MaxPlayers))
	memoryPlan := m.MemoryPlan()
	memoryModeLabel := widget.NewLabel(loc.T("memory_mode"))
	memoryLimitLabel := widget.NewLabel(loc.T("memory_limit"))
	memoryLimitEntry := widget.NewEntry()
	memoryLimit := settings.MemoryLimitGB
	if memoryLimit < 1 {
		memoryLimit = memoryPlan.RecommendedGB
	}
	memoryLimitEntry.SetText(strconv.Itoa(memoryLimit))
	memoryDetectedLabel := widget.NewLabel(loc.T("memory_detected", memoryPlan.TotalGB, memoryPlan.RecommendedGB))
	memoryDetectedLabel.Wrapping = fyne.TextWrapWord
	memoryHelp := widget.NewLabel(loc.T("memory_help"))
	memoryHelp.Wrapping = fyne.TextWrapWord
	memoryRestartHelp := widget.NewLabel(loc.T("memory_restart"))
	memoryRestartHelp.Wrapping = fyne.TextWrapWord
	changingMemoryMode := false
	selectedMemoryMode := settings.MemoryMode
	if selectedMemoryMode != manager.MemoryModeManual {
		selectedMemoryMode = manager.MemoryModeAutomatic
	}
	memoryModeSelect := widget.NewSelect(nil, func(display string) {
		if changingMemoryMode {
			return
		}
		if display == loc.T("memory_manual") {
			selectedMemoryMode = manager.MemoryModeManual
			memoryLimitEntry.Enable()
		} else {
			selectedMemoryMode = manager.MemoryModeAutomatic
			memoryLimitEntry.SetText(strconv.Itoa(memoryPlan.RecommendedGB))
			memoryLimitEntry.Disable()
		}
	})
	setMemoryModeOptions := func() {
		changingMemoryMode = true
		memoryModeSelect.Options = []string{loc.T("memory_auto"), loc.T("memory_manual")}
		if selectedMemoryMode == manager.MemoryModeManual {
			memoryModeSelect.SetSelected(loc.T("memory_manual"))
			memoryLimitEntry.Enable()
		} else {
			memoryModeSelect.SetSelected(loc.T("memory_auto"))
			memoryLimitEntry.Disable()
		}
		memoryModeSelect.Refresh()
		changingMemoryMode = false
	}
	setMemoryModeOptions()
	languageLabel := widget.NewLabel(loc.T("language"))
	changingLanguage := false
	var retranslate func()
	languageSelect := widget.NewSelect([]string{loc.T("english"), loc.T("portuguese")}, func(display string) {
		if changingLanguage {
			return
		}
		language := i18n.English
		if display == "Português (Brasil)" {
			language = i18n.Portuguese
		}
		loc.SetLanguage(language)
		m.SetLanguage(language)
		if retranslate != nil {
			retranslate()
		}
	})
	saveSettingsButton := widget.NewButtonWithIcon(loc.T("save_settings"), theme.DocumentSaveIcon(), func() {
		serverName := strings.TrimSpace(serverNameEntry.Text)
		maxPlayers, err := strconv.Atoi(strings.TrimSpace(maxPlayersEntry.Text))
		if serverName == "" || err != nil || maxPlayers < 1 || maxPlayers > 1000 {
			showInfo("settings_failed", "invalid_settings")
			return
		}
		memoryMode := selectedMemoryMode
		memoryLimitGB := 0
		if memoryMode == manager.MemoryModeManual {
			memoryLimitGB, err = strconv.Atoi(strings.TrimSpace(memoryLimitEntry.Text))
			if err != nil {
				showInfo("settings_failed", "invalid_settings")
				return
			}
		}
		go func() {
			if err := m.SaveSettingsWithMemory(loc.Language(), serverName, maxPlayers, memoryMode, memoryLimitGB); err != nil {
				showError("settings_failed", err)
				return
			}
			showInfo("settings_saved", "settings_saved_message")
		}()
	})
	settingsCard := widget.NewCard(loc.T("settings_title"), loc.T("settings_subtitle"), container.NewVBox(
		serverNameLabel, serverNameEntry, maxPlayersLabel, maxPlayersEntry, widget.NewSeparator(),
		memoryModeLabel, memoryModeSelect, memoryLimitLabel, memoryLimitEntry, memoryDetectedLabel, memoryHelp, memoryRestartHelp,
		widget.NewSeparator(), languageLabel, languageSelect, saveSettingsButton,
	))
	openFolderButton := widget.NewButtonWithIcon(loc.T("open_folder"), theme.FolderOpenIcon(), func() {
		u.runSimple("open_failed", func(context.Context) error { return openFolder(m.ServerDir()) }, showError)
	})
	refreshFilesButton := widget.NewButtonWithIcon(loc.T("refresh_list"), theme.ViewRefreshIcon(), func() { go refreshFiles() })
	fileSplit := container.NewHSplit(container.NewVScroll(filesBox), filePreview)
	fileSplit.Offset = 0.42
	filesCard := widget.NewCard(loc.T("files_title"), loc.T("files_subtitle"), container.NewBorder(container.NewHBox(openFolderButton, refreshFilesButton, layout.NewSpacer()), nil, nil, nil, fileSplit))
	settingsAndFiles := container.NewVSplit(container.NewVScroll(settingsCard), filesCard)
	settingsAndFiles.Offset = 0.36

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon(loc.T("home_tab"), theme.HomeIcon(), home),
		container.NewTabItemWithIcon(loc.T("console_tab"), theme.ComputerIcon(), terminal),
		container.NewTabItemWithIcon(loc.T("players_tab"), theme.AccountIcon(), playerManager),
		container.NewTabItemWithIcon(loc.T("controls_tab"), theme.SettingsIcon(), controls),
		container.NewTabItemWithIcon(loc.T("settings_tab"), theme.FolderIcon(), settingsAndFiles),
	)
	setCardText := func(card *widget.Card, titleKey, subtitleKey string) {
		card.Title, card.Subtitle = loc.T(titleKey), loc.T(subtitleKey)
		card.Refresh()
	}
	retranslate = func() {
		keys := []string{"home_tab", "console_tab", "players_tab", "controls_tab", "settings_tab"}
		for index, key := range keys {
			tabs.Items[index].Text = loc.T(key)
		}
		tabs.Refresh()
		playButton.SetText(loc.T("play"))
		stopButton.SetText(loc.T("stop_server"))
		restartButton.SetText(loc.T("restart_server"))
		updateButton.SetText(loc.T("update_dependencies"))
		sendButton.SetText(loc.T("send"))
		commandEntry.SetPlaceHolder(loc.T("command_placeholder"))
		console.SetPlaceHolder(loc.T("logs_placeholder"))
		filePreview.SetPlaceHolder(loc.T("file_placeholder"))
		homeHelp.SetText(loc.T("quick_guide"))
		setCardText(jvmCard, "jvm_title", "jvm_subtitle")
		setCardText(networkCard, "network_title", "network_subtitle")
		setCardText(overviewCard, "overview_title", "overview_subtitle")
		setCardText(catalogCard, "command_catalog", "command_catalog_subtitle")
		commandSearch.SetPlaceHolder(loc.T("search_commands"))
		refreshCommandCatalog()
		linkControls.Retranslate()
		setCardText(onlineCard, "online_players", "quick_actions")
		setCardText(offlineCard, "offline_players", "known_players")
		setCardText(bansCard, "banned_players", "ban_details")
		setCardText(versionCard, "minecraft_version", "version_subtitle")
		setCardText(generalCard, "general", "general_subtitle")
		setCardText(weatherCard, "weather", "weather_subtitle")
		setCardText(timeCard, "time", "time_subtitle")
		setCardText(difficultyCard, "difficulty", "difficulty_subtitle")
		setCardText(gamemodeCard, "gamemode", "gamemode_subtitle")
		setCardText(settingsCard, "settings_title", "settings_subtitle")
		setCardText(filesCard, "files_title", "files_subtitle")
		if !installedLoaded {
			installedLabel.SetText(loc.T("installed_version", loc.T("loading")))
		} else if installedValue == "" {
			installedLabel.SetText(loc.T("installed_version", loc.T("unknown")))
		} else {
			installedLabel.SetText(loc.T("installed_version", installedValue))
		}
		applyVersionButton.SetText(loc.T("apply_version"))
		refreshVersionsButton.SetText(loc.T("refresh_list"))
		versionWarning.SetText(loc.T("version_warning"))
		saveWorldButton.SetText(loc.T("save_world"))
		clearButton.SetText(loc.T("clear"))
		rainButton.SetText(loc.T("rain"))
		thunderButton.SetText(loc.T("thunder"))
		dayButton.SetText(loc.T("day"))
		nightButton.SetText(loc.T("night"))
		peacefulButton.SetText(loc.T("peaceful"))
		easyButton.SetText(loc.T("easy"))
		normalButton.SetText(loc.T("normal"))
		hardButton.SetText(loc.T("hard"))
		survivalButton.SetText(loc.T("survival"))
		creativeButton.SetText(loc.T("creative"))
		playerName.SetPlaceHolder(loc.T("player_name"))
		cancelProgressButton.SetText(loc.T("cancel"))
		serverNameLabel.SetText(loc.T("server_name"))
		serverNameEntry.SetPlaceHolder(loc.T("server_name_placeholder"))
		maxPlayersLabel.SetText(loc.T("max_players"))
		maxPlayersEntry.SetPlaceHolder(loc.T("max_players_placeholder"))
		memoryModeLabel.SetText(loc.T("memory_mode"))
		memoryLimitLabel.SetText(loc.T("memory_limit"))
		memoryDetectedLabel.SetText(loc.T("memory_detected", memoryPlan.TotalGB, memoryPlan.RecommendedGB))
		memoryHelp.SetText(loc.T("memory_help"))
		memoryRestartHelp.SetText(loc.T("memory_restart"))
		setMemoryModeOptions()
		languageLabel.SetText(loc.T("language"))
		saveSettingsButton.SetText(loc.T("save_settings"))
		openFolderButton.SetText(loc.T("open_folder"))
		refreshFilesButton.SetText(loc.T("refresh_list"))
		changingLanguage = true
		languageSelect.Options = []string{loc.T("english"), loc.T("portuguese")}
		if loc.Language() == i18n.English {
			languageSelect.SetSelected(loc.T("english"))
		} else {
			languageSelect.SetSelected(loc.T("portuguese"))
		}
		changingLanguage = false
		setVersionOptions()
		refreshStatus(lastStatus)
		go refreshPlayers()
		go refreshFiles()
	}

	m.SetEvents(manager.Events{
		Log: refreshConsole, Status: refreshStatus,
		Players: func() { go refreshPlayers() }, Files: func() { go refreshFiles() },
		Language: func(language string) { fyne.Do(func() { loc.SetLanguage(language); retranslate() }) },
		Settings: func(saved manager.Settings) {
			fyne.Do(func() {
				serverNameEntry.SetText(saved.ServerName)
				maxPlayersEntry.SetText(strconv.Itoa(saved.MaxPlayers))
				selectedMemoryMode = saved.MemoryMode
				if selectedMemoryMode != manager.MemoryModeManual {
					selectedMemoryMode = manager.MemoryModeAutomatic
				}
				memoryPlan = m.MemoryPlan()
				limit := saved.MemoryLimitGB
				if limit < 1 {
					limit = memoryPlan.RecommendedGB
				}
				memoryLimitEntry.SetText(strconv.Itoa(limit))
				memoryDetectedLabel.SetText(loc.T("memory_detected", memoryPlan.TotalGB, memoryPlan.RecommendedGB))
				setMemoryModeOptions()
			})
		},
		Progress: func(progress manager.Progress) {
			fyne.Do(func() {
				if progress.Done {
					progressBar.SetValue(0)
					progressLabel.SetText(loc.T("progress_idle"))
					cancelProgressButton.Disable()
					return
				}
				cancelProgressButton.Enable()
				progressLabel.SetText(loc.T("progress_working", progress.Task))
				value := 0.0
				if progress.Total > 0 {
					value = float64(progress.Downloaded) / float64(progress.Total)
				} else if progress.TotalTasks > 0 {
					value = float64(progress.Completed) / float64(progress.TotalTasks)
				}
				progressBar.SetValue(value)
			})
		},
	})
	retranslate()
	go func() {
		if err := m.Initialize(); err != nil {
			m.Log("error", "startup", "falha na inicialização", err)
			showError("start_failed", err)
		}
	}()
	go loadVersions()
	go func() {
		installed := m.InstalledPaperVersion()
		status := m.Status()
		fyne.Do(func() {
			installedValue = installed
			installedLoaded = true
			display := installed
			if display == "" {
				display = loc.T("unknown")
			}
			installedLabel.SetText(loc.T("installed_version", display))
			linkControls.Update(status, installedValue)
		})
	}()
	return tabs
}

func (u *UI) runSimple(errorKey string, operation func(context.Context) error, showError func(string, error)) {
	go func() {
		if err := operation(u.manager.Context()); err != nil {
			showError(errorKey, err)
		}
	}()
}

func (u *UI) runOperation(operation func(context.Context) error, completed func(error), cancelButton *widget.Button) {
	u.opMu.Lock()
	if u.busy {
		u.opMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(u.manager.Context())
	u.busy = true
	u.opCancel = cancel
	u.opMu.Unlock()
	fyne.Do(cancelButton.Enable)
	go func() {
		err := operation(ctx)
		cancel()
		u.opMu.Lock()
		u.busy = false
		u.opCancel = nil
		u.opMu.Unlock()
		fyne.Do(cancelButton.Disable)
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
