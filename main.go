package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"soundshift/fyneTheme"
	"soundshift/interfaces/mmDeviceEnumerator"
	"soundshift/interfaces/policyConfig"
	"soundshift/winapi"
	"time"

	"github.com/energye/systray"
	"github.com/lxn/win"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type DeviceConfig struct {
	Name    string
	IsShown bool
}

type AppSettings struct {
	HideAfterSelection bool
	DeviceNames        map[string]DeviceConfig
}

//go:embed speaker.ico
var icon []byte
var title = "SoundShift"
var App fyne.App = app.NewWithID(title)
var Win fyne.Window = App.NewWindow(title)
var configWin fyne.Window = App.NewWindow("Configure")
var vbox = container.NewVBox()
var mainView = container.NewCenter(container.NewPadded(vbox))

var configureButton *widget.Button
var settings AppSettings

var screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
var screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
var taskbarHeight = winapi.GetTaskbarHeight()

func main() {
	settings.DeviceNames = make(map[string]DeviceConfig)

	Win.SetContent(mainView)
	Win.SetTitle(title)
	Win.SetIcon(fyne.NewStaticResource("icon", icon))
	Win.Resize(fyne.NewSize(250, 300))
	Win.SetFixedSize(true)
	App.Settings().SetTheme(fyneTheme.CustomTheme{})
	go systray.Run(initTray, func() {})
	go func() {
		for {
			time.Sleep(250 * time.Millisecond)
			winapi.DisableMinMaxButtons(title)
			winapi.DisableCloseButton(title)
			winapi.HideWindowFromTaskbar(title)
			winapi.SetWindowAlwaysOnTop(title)

			size := Win.Canvas().Size()
			winapi.MoveWindow(title, int32(screenWidth-int(size.Width)-20), int32(screenHeight-int(size.Height)-45-taskbarHeight), int32(size.Width), int32(size.Height))
		}
	}()

	loadSettings()
	renderButtons()
	Win.ShowAndRun()
}

func renderButtons() {
	audioDevices := mmDeviceEnumerator.GetDevices()

	//. Create audio device buttons
	vbox.Objects = nil
	for i := 0; i < len(audioDevices); i++ {
		index := i
		config, exists := settings.DeviceNames[audioDevices[i].Id]
		if !exists {
			//* No override found, use default name
			config = DeviceConfig{
				Name:    audioDevices[i].Name,
				IsShown: true,
			}
		}

		if !config.IsShown {
			//* Device hidden, do not render
			continue
		}

		if audioDevices[i].IsDefault {
			//* Default audio device
			vbox.Add(widget.NewButtonWithIcon(config.Name, theme.VolumeUpIcon(), func() {
				policyConfig.SetDefaultEndPoint(audioDevices[index].Id)
				renderButtons()
				if settings.HideAfterSelection {
					winapi.HideWindow(title)
				}
			}))
		} else {
			//* Not default audio device
			vbox.Add(&widget.Button{
				Text: config.Name,
				OnTapped: func() {
					policyConfig.SetDefaultEndPoint(audioDevices[index].Id)
					renderButtons()
					if settings.HideAfterSelection {
						winapi.HideWindow(title)
					}
				},
			})
		}
	}

	//* Create spacer
	vbox.Add(&widget.Label{
		Text: "                                       ",
	})

	//* Create configure button
	configureButton = &widget.Button{
		Text: "Configure",
		OnTapped: func() {
			configureButton.Disable()

			if configWin != nil {
				configWin.Close()
			}

			configWin = App.NewWindow("Configure")
			configWin.SetContent(genConfigForm())
			configWin.SetOnClosed(func() {
				configureButton.Enable()
			})
			configWin.CenterOnScreen()
			configWin.Show()
		},
	}

	vbox.Add(configureButton)

	//* Create hide button
	vbox.Add(&widget.Button{
		Text: "Hide",
		OnTapped: func() {
			winapi.HideWindow(title)
		},
	})

	mainView.Refresh()
}

func saveSettings() {
	file, err := json.Marshal(settings)
	if err != nil {
		//lint:ignore ST1005 Will not be logged to a console
		errorDialog := dialog.NewError(fmt.Errorf("Error saving settings: %s", err), Win)
		errorDialog.Show()
		return
	}

	err = os.WriteFile("settings.json", file, 0644)
	if err != nil {
		//lint:ignore ST1005 Will not be logged to a console
		errorDialog := dialog.NewError(fmt.Errorf("Error saving settings: %s", err), Win)
		errorDialog.Show()
	}
}

func loadSettings() {
	file, err := os.ReadFile("settings.json")
	if err != nil {
		//lint:ignore ST1005 Will not be logged to a console
		errorDialog := dialog.NewError(fmt.Errorf("Error loading settings: %s", err), Win)
		errorDialog.Show()
		return
	}

	err = json.Unmarshal(file, &settings)
	if err != nil {
		//lint:ignore ST1005 Will not be logged to a console
		errorDialog := dialog.NewError(fmt.Errorf("Error loading settings: %s", err), Win)
		errorDialog.Show()
		return
	}

	if settings.DeviceNames == nil {
		settings.DeviceNames = make(map[string]DeviceConfig)
	}
}

func genConfigForm() fyne.CanvasObject {
	form := &widget.Form{}

	audioDevices := mmDeviceEnumerator.GetDevices()

	for i := 0; i < len(audioDevices); i++ {
		deviceName := audioDevices[i].Name
		newNameEntry := widget.NewEntry()
		showHideCheckbox := widget.NewCheck("Shown                      ", nil)

		config, exists := settings.DeviceNames[audioDevices[i].Id]
		if !exists {
			config = DeviceConfig{
				Name:    audioDevices[i].Name,
				IsShown: true,
			}
		}
		newNameEntry.SetText(config.Name)
		showHideCheckbox.SetChecked(config.IsShown)

		form.Append(deviceName, newNameEntry)
		form.Append("", showHideCheckbox)
	}

	hideAfterSelectionCheckbox := widget.NewCheck("Hide after selection", nil)
	hideAfterSelectionCheckbox.SetChecked(settings.HideAfterSelection)

	saveButton := widget.NewButton("     Save     ", func() {
		for i := 0; i < len(audioDevices); i++ {
			newNameEntry := form.Items[i*2].Widget.(*widget.Entry)
			showHideCheckbox := form.Items[i*2+1].Widget.(*widget.Check)

			settings.DeviceNames[audioDevices[i].Id] = DeviceConfig{
				Name:    newNameEntry.Text,
				IsShown: showHideCheckbox.Checked,
			}

			settings.HideAfterSelection = hideAfterSelectionCheckbox.Checked
		}

		saveSettings()
		renderButtons()
		configWin.Close()
	})

	saveButtonContainer := container.New(layout.NewCenterLayout(), saveButton)
	checkboxAndButtonVBox := container.NewVBox(hideAfterSelectionCheckbox, saveButtonContainer)
	centeredCheckboxAndButtonContainer := container.New(layout.NewCenterLayout(), checkboxAndButtonVBox)
	borderContainer := container.NewBorder(form, centeredCheckboxAndButtonContainer, nil, nil, nil)
	return borderContainer
}

func initTray() {
	systray.SetIcon(icon)
	systray.SetTitle(title)
	systray.SetTooltip(title)
	systray.SetOnClick(func() {
		winapi.ShowWindow(title)
	})

	mQuit := systray.AddMenuItem("Exit", "Completely exit SoundShift")
	mQuit.Enable()
	mQuit.Click(func() {
		os.Exit(0)
	})
}
