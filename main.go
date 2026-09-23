package main

import (
	"os"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"mineserver/internal/gui"
	"mineserver/internal/manager"
)

const (
	appID    = "io.github.pingu.minecraft.server.manager"
	appTitle = "Pingu"
)

var appVersion = "development"

func main() {
	application := app.NewWithID(appID)
	application.Settings().SetTheme(theme.DarkTheme())
	window := application.NewWindow(appTitle)
	window.Resize(fyne.NewSize(1180, 760))
	window.SetMaster()

	service := manager.New(applicationDataDirectory())
	userInterface := gui.New(application, window, service)
	window.SetContent(userInterface.Build())

	window.SetCloseIntercept(func() {
		window.SetCloseIntercept(nil)
		go func() {
			if err := service.Close(); err != nil {
				service.Log("error", "shutdown", "falha durante encerramento", err)
			}
			fyne.Do(window.Close)
		}()
	})
	window.ShowAndRun()
}

func applicationDataDirectory() string {
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, "Pingu Minecraft Server Manager")
		}
		if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
			return filepath.Join(configDir, "Pingu Minecraft Server Manager")
		}
	}
	return "."
}
