package main

import (
	_ "embed"
	"encoding/json"
	"image/color"
	"os"
	"soundshift/colormap"
	"soundshift/file"
	"soundshift/fyneCustom"
	"soundshift/fyneTheme"
	"soundshift/general"
	"soundshift/interfaces/mmDeviceEnumerator"
	"soundshift/interfaces/policyConfig"
	"soundshift/winapi"
	"time"

	"github.com/energye/systray"
	"github.com/go-vgo/robotgo"
	"github.com/lxn/win"
	"github.com/moutend/go-hook/pkg/mouse"
	"github.com/moutend/go-hook/pkg/types"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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

var initialized = false
var configWindowOpen = false
var settings AppSettings
var lastInteractionTime time.Time
var debounceDuration = 100 * time.Millisecond

var screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
var screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
var taskbarHeight = winapi.GetTaskbarHeight()

//go:embed speaker.ico
var icon []byte
var title = "SoundShift "
var App fyne.App = app.NewWithID(title)
var Win fyne.Window = App.NewWindow(title)
var configWin fyne.Window = App.NewWindow("Configure")

// var deviceVbox = container.NewVBox()

var deviceVbox = container.New(&fyneCustom.CustomVBoxLayout{FixedWidth: 150})
var mainView = container.NewCenter(
	container.NewPadded(
		container.NewVBox(
			deviceVbox,
			&canvas.Line{StrokeColor: colormap.Gray, StrokeWidth: 1},
			configureButton,
		),
	),
)

var configureButton = &widget.Button{Text: "Configure"}

func init() {
	configureButton.OnTapped = func() {
		configureButton.Disable()
		configWindowOpen = true

		if configWin != nil {
			configWin.Close()
		}

		configWin = App.NewWindow("Configure")
		configWin.SetIcon(fyne.NewStaticResource("icon", icon))
		configWin.SetContent(genConfigForm())
		configWin.Resize(fyne.NewSize(600, 500))
		configWin.SetOnClosed(func() {
			configureButton.Enable()
			configWindowOpen = false
		})
		configWin.CenterOnScreen()
		configWin.Show()
	}
	configureButton.Refresh()
}

func main() {
	if general.IsProcRunning(title) {
		//! SoundShift is already running, exit
		os.Exit(0)
	}

	Win.SetContent(mainView)
	Win.SetTitle(title)
	Win.SetIcon(fyne.NewStaticResource("icon", icon))
	configWin.SetIcon(fyne.NewStaticResource("icon", icon))
	Win.Resize(fyne.NewSize(250, 300))
	Win.SetFixedSize(true)
	Win.SetCloseIntercept(func() {
		winapi.HideWindow(title)
	})
	App.Settings().SetTheme(fyneTheme.CustomTheme{})
	App.Lifecycle().SetOnEnteredForeground(func() {
		if !initialized {
			winapi.HideWindow(title)
			winapi.HideMinMaxButtons(title)
			winapi.HideWindowFromTaskbar(title)
			winapi.SetTopmost(title)

			size := Win.Canvas().Size()
			winapi.MoveWindow(title, int32(screenWidth-int(size.Width)-20), int32(screenHeight-int(size.Height)-45-taskbarHeight), int32(size.Width), int32(size.Height))
			initialized = true
		}
	})

	loadSettings()
	renderButtons()
	go hideOnClick()
	go updateDevices()
	go systray.Run(initTray, func() {})
	Win.ShowAndRun()
}

func updateDevices() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		go renderButtons()
	}
}

func hideOnClick() {
	mouseChan := make(chan types.MouseEvent)
	mouse.Install(nil, mouseChan)
	defer mouse.Uninstall()

	for k := range mouseChan {
		if k.Message == 513 {
			if !isMouseInWindow() && !isMouseInTaskbar() && !configWindowOpen {
				lastInteractionTime = time.Now()
				go debounceHideWindow()
			}
		}
	}
}

func debounceHideWindow() {
	time.Sleep(debounceDuration)
	if time.Since(lastInteractionTime) >= debounceDuration && winapi.IsWindowVisible(title) {
		winapi.HideWindow(title)
	}
}

func isMouseInWindow() bool {
	xMouse, yMouse := robotgo.Location()
	xPos, yPos, _ := winapi.GetWindowPosition(title)
	xSize, ySize, _ := winapi.GetWindowSize(title)
	return int(xMouse) >= int(xPos) && int(xMouse) <= int(xPos+xSize) && int(yMouse) >= int(yPos) && int(yMouse) <= int(yPos+ySize)
}

func isMouseInTaskbar() bool {
	_, yMouse := robotgo.Location()
	return screenHeight-yMouse <= winapi.GetTaskbarHeight()
}

func renderButtons() {
	//* Get audio devices
	audioDevices := mmDeviceEnumerator.GetDevices()

	//* Reset deviceVbox
	deviceVbox.Objects = nil

	//* Create button for each audio device
	for i := range audioDevices {
		//* Get device config
		device := audioDevices[i]
		config, exists := settings.DeviceNames[device.Id]
		if !exists {
			//. Config did not exist, creating a new one
			config = DeviceConfig{Name: device.Name, IsShown: true}
		}

		if !config.IsShown {
			//! Device is hidden, skip button creation
			continue
		}

		//* Get device name
		deviceName := general.EllipticalTruncate(config.Name, 20)

		//* Create button tapped function
		onTapped := func() {
			policyConfig.SetDefaultEndPoint(device.Id)
			go renderButtons()
			if settings.HideAfterSelection {
				winapi.HideWindow(title)
			}
		}

		if device.IsDefault {
			//* Add default audio device button
			deviceVbox.Add(widget.NewButtonWithIcon(deviceName, theme.VolumeUpIcon(), onTapped))
		} else {
			//* Add non-default audio device button
			deviceVbox.Add(&widget.Button{Text: deviceName, OnTapped: onTapped})
		}
	}

	//* Update configure button state
	if configWindowOpen {
		configureButton.Disable()
		configWin.SetContent(genConfigForm())
	}

	for i := range deviceVbox.Objects {
		deviceVbox.Objects[i].Refresh()
	}
}

func loadSettings() {
	settingsPath := file.RoamingDir() + "/soundshift/settings.json"

	//* Initialize settings with default values
	settings = AppSettings{
		HideAfterSelection: false,
		DeviceNames:        make(map[string]DeviceConfig),
	}

	//* Read settings file
	fileData, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			//. Settings file does not exist, creating a new one
			saveSettings()
		}
		return
	}

	//* Parse settings from file data
	json.Unmarshal(fileData, &settings)

	//* Ensure DeviceNames is initialized
	if settings.DeviceNames == nil {
		settings.DeviceNames = make(map[string]DeviceConfig)
	}
}

func saveSettings() {
	fileData, _ := json.Marshal(settings)
	os.MkdirAll(file.RoamingDir()+"/soundshift", os.ModePerm)
	os.WriteFile(file.RoamingDir()+"/soundshift/settings.json", fileData, 0644)
}

func genConfigForm() fyne.CanvasObject {
	//* Get audio devices
	audioDevices := mmDeviceEnumerator.GetDevices()

	//* Create form
	form := &widget.Form{}
	for _, device := range audioDevices {
		//* Get device config
		config, exists := settings.DeviceNames[device.Id]
		if !exists {
			//. Config did not exist,using default
			config = DeviceConfig{
				Name:    device.Name,
				IsShown: true,
			}
		}

		//* Create new name entry
		newNameEntry := &widget.Entry{
			PlaceHolder: "Device Name",
			ActionItem:  fyneCustom.NewColorButton("", color.RGBA{68, 72, 81, 255}, theme.MediaReplayIcon(), func() {}),
			Text:        config.Name,
		}

		//* Create show/hide checkbox
		showHideCheckbox := &widget.Check{
			Text:    "Shown",
			Checked: config.IsShown,
		}

		//* Append elements to the form
		form.Append(device.Name, newNameEntry)
		form.Append("", showHideCheckbox)
	}

	//* Create hide after selection checkbox
	hideAfterSelectionCheckbox := &widget.Check{
		Text:    "Hide after selection",
		Checked: settings.HideAfterSelection,
	}

	//* Create start with windows checkbox
	startWithWindowsCheckbox := &widget.Check{
		Text:    "Start with Windows",
		Checked: file.Exists(file.RoamingDir() + "/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk"),
	}

	//* Create save button
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

		if startWithWindowsCheckbox.Checked {
			if !file.Exists(file.RoamingDir() + "/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk") {
				general.CreateShortcut(file.Cwd()+"/soundshift.exe", file.RoamingDir()+"/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk")
			}
		} else {
			if file.Exists(file.RoamingDir() + "/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk") {
				os.Remove(file.RoamingDir() + "/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk")
			}
		}

		saveSettings()
		configWin.Close()
		go renderButtons()
	})

	saveButtonContainer := container.New(layout.NewCenterLayout(), saveButton)
	checkboxAndButtonVBox := container.NewVBox(hideAfterSelectionCheckbox, startWithWindowsCheckbox, saveButtonContainer)
	centeredCheckboxAndButtonContainer := container.New(layout.NewCenterLayout(), checkboxAndButtonVBox)
	borderContainer := container.NewPadded(container.NewBorder(form, centeredCheckboxAndButtonContainer, nil, nil, nil))
	return borderContainer
}

func initTray() {
	systray.SetIcon(icon)
	systray.SetTitle(title)
	systray.SetTooltip(title)
	systray.SetOnClick(func(menu systray.IMenu) {
		if winapi.IsWindowVisible(title) {
			winapi.HideWindow(title)
		} else {
			size := Win.Canvas().Size()
			winapi.MoveWindow(title, int32(screenWidth-int(size.Width)-20), int32(screenHeight-int(size.Height)-45-taskbarHeight), int32(size.Width), int32(size.Height))
			winapi.ShowWindow(title)
			winapi.SetTopmost(title)
		}
	})

	mQuit := systray.AddMenuItem("Exit", "Completely exit SoundShift")
	mQuit.Enable()
	mQuit.Click(func() {
		os.Exit(0)
	})
}
