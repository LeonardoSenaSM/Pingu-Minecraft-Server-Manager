package main

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	"mineserver/internal/gui"
	"mineserver/internal/manager"
)

const (
	appID    = "io.github.pingu.app"
	appTitle = "Pingu - Minecraft Server Manager"
)

func main() {
	application := app.NewWithID(appID)
	application.Settings().SetTheme(theme.DarkTheme())
	iconRes, err := fyne.LoadResourceFromPath("assets/icon.png")
	if err != nil {
		log.Printf("Aviso: Não foi possível carregar o ícone: %v", err)
	} else {
		application.SetIcon(iconRes)
	}

	window := application.NewWindow(appTitle)
	window.Resize(fyne.NewSize(1180, 760))
	window.SetMaster()

	if iconRes != nil {
		window.SetIcon(iconRes)
	}

	service := manager.New(".")
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
