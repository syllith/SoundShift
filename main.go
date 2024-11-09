package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"reflect"
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
	"github.com/go-ole/go-ole"
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

// . Structs
type DeviceConfig struct {
	Name         string
	IsShown      bool
	OriginalName string
}

type AppSettings struct {
	HideAfterSelection bool
	DeviceNames        map[string]DeviceConfig
}

// . Globals
var initialized = false
var configWindowOpen = false
var settings AppSettings
var lastInteractionTime time.Time
var debounceDuration = 100 * time.Millisecond
var currentDeviceID string
var audioDevices []mmDeviceEnumerator.AudioDevice
var screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
var screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
var taskbarHeight = winapi.GetTaskbarHeight()

//go:embed speaker.ico
var icon []byte
var title = "SoundShift "
var App fyne.App = app.NewWithID(title)
var Win fyne.Window = App.NewWindow(title)
var configWin fyne.Window = App.NewWindow("Configure")

// * Device vbox
var deviceVbox = container.New(&fyneCustom.CustomVBoxLayout{FixedWidth: 150})

// * Main view
var mainView = container.NewCenter(
	container.NewPadded(
		container.NewVBox(
			deviceVbox,
			&canvas.Line{StrokeColor: colormap.Gray, StrokeWidth: 1},
			configButton,
			&canvas.Text{Text: "", TextSize: 10},
			container.NewPadded(volumeSlider),
		),
	),
)

// * Config button
var configButton = &widget.Button{Text: "Configure"}

// * Volume slider
var volumeSlider = fyneCustom.NewScrollableSlider(0, 100)

// . Initialization
func init() {
	//* Voume slider on changed
	volumeSlider.OnChanged = func(f float64) {
		volumeScalar := float32(f / 100.0)
		if currentDeviceID != "" {
			if err := policyConfig.SetVolume(currentDeviceID, volumeScalar); err != nil {
				fmt.Println("Error setting volume:", err)
				general.LogError("Error setting volume:", err)
			}
		}
	}

	configWin = fyne.CurrentApp().NewWindow("Configure")
	configButton.OnTapped = func() {
		if !configWindowOpen {
			configWin.Show()
			configWin.CenterOnScreen()
			configWindowOpen = true
			configButton.Disable()
		}
	}
	configWin.SetCloseIntercept(func() {
		configWin.Hide()
		configWindowOpen = false
		configButton.Enable()
	})
}

// . Main
func main() {
	if general.IsProcRunning(title) {
		//! SoundShift is already running, exit
		os.Exit(0)
	}

	//* Load audio device settings
	loadSettings()

	//* Initialize COM library
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		fmt.Println("Failed to initialize COM library:", err)
		return
	}
	defer ole.CoUninitialize()

	//* Configure application
	App.Settings().SetTheme(fyneTheme.CustomTheme{})
	App.Lifecycle().SetOnEnteredForeground(func() {
		if !initialized {
			//* Setup window
			winapi.HideWindow(title)
			winapi.HideMinMaxButtons(title)
			winapi.HideWindowFromTaskbar(title)
			winapi.SetTopmost(title)

			//* Relocate window
			size := Win.Canvas().Size()
			winapi.MoveWindow(title, int32(screenWidth-int(size.Width)-20), int32(screenHeight-int(size.Height)-45-taskbarHeight), int32(size.Width), int32(size.Height))
			initialized = true
		}
	})

	//* Configure main window
	Win.SetContent(mainView)
	Win.SetTitle(title)
	Win.SetIcon(fyne.NewStaticResource("icon", icon))
	Win.Resize(fyne.NewSize(250, 300))
	//Win.SetFixedSize(true)
	Win.SetCloseIntercept(func() {
		winapi.HideWindow(title)
	})

	//* Configure config window
	configWin.SetIcon(fyne.NewStaticResource("icon", icon))
	configWin.SetContent(genConfigForm())
	configWin.Resize(fyne.NewSize(600, 500))
	configWin.SetOnClosed(func() {
		configButton.Enable()
		configWindowOpen = false
	})

	go hideOnClick()
	go updateDevices()
	go systray.Run(initTray, func() {})
	Win.ShowAndRun()
}

// . Check and update audio devices
func checkAndUpdateDevices() {
	newAudioDevices, err := mmDeviceEnumerator.GetDevices()
	if err != nil || newAudioDevices == nil {
		fmt.Println("Error getting audio devices:", err)
		general.LogError("Error getting audio devices:", err)
		return
	}

	if !slicesEqual(audioDevices, newAudioDevices) && !configWindowOpen {
		//. Audio devices changed
		audioDevices = newAudioDevices
		go renderButtons()
	}
}

// . Loop to update audio devices
func updateDevices() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	checkAndUpdateDevices()

	for range ticker.C {
		checkAndUpdateDevices()
	}
}

// . Render audio device buttons
func renderButtons() {
	//* Reset deviceVbox
	deviceVbox.Objects = nil

	//* Create button for each audio device
	for i := range audioDevices {
		//* Get device config
		device := audioDevices[i]
		config, exists := settings.DeviceNames[device.Id]
		if !exists {
			//. Config did not exist, using default
			config = DeviceConfig{Name: device.Name, IsShown: true}
		}

		if !config.IsShown {
			//! Device is hidden, skip button creation
			continue
		}

		//* Get device name
		deviceName := general.EllipticalTruncate(config.Name, 15)

		//* Create button tapped function
		onTapped := func() {
			//* Set default audio device
			err := policyConfig.SetDefaultEndPoint(device.Id)
			if err != nil {
				fmt.Println("Error setting default endpoint:", err)
				general.LogError("Error setting default endpoint:", err)
				return
			}

			//* Hide window if setting is enabled
			if settings.HideAfterSelection {
				winapi.HideWindow(title)
			}

			// * Update buttons to reflect new default device
			for i := range audioDevices {
				if audioDevices[i].Id == device.Id {
					audioDevices[i].IsDefault = true
				} else {
					audioDevices[i].IsDefault = false
				}
			}

			deviceVbox.Refresh()
			go renderButtons()
		}

		//* Add button to deviceVbox
		if device.IsDefault {
			//* Add default audio device button
			deviceVbox.Add(widget.NewButtonWithIcon(deviceName, theme.VolumeUpIcon(), onTapped))
			currentDeviceID = device.Id
			volume, err := policyConfig.GetVolume(currentDeviceID)
			if err != nil {
				fmt.Println("Error getting volume:", err)
			} else {
				volumeSlider.SetValue(float64(volume * 100)) // Convert volume scalar to percentage
			}
		} else {
			//* Add non-default audio device button
			deviceVbox.Add(&widget.Button{Text: deviceName, OnTapped: onTapped})
		}
	}

	//* Refresh device buttons
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

// . Save settings to file
func saveSettings() {
	fileData, _ := json.MarshalIndent(settings, "", "    ")
	os.MkdirAll(file.RoamingDir()+"/soundshift", os.ModePerm)
	os.WriteFile(file.RoamingDir()+"/soundshift/settings.json", fileData, 0644)
}

// . Generate configuration form
func genConfigForm() fyne.CanvasObject {
	//* Get audio devices
	audioDevices, err := mmDeviceEnumerator.GetDevices()
	if err != nil || audioDevices == nil {
		fmt.Println("Error getting audio devices:", err)
		general.LogError("Error getting audio devices:", err)
		return nil
	}

	//* Create form
	form := &widget.Form{}
	for _, device := range audioDevices {
		//* Get device config
		config, exists := settings.DeviceNames[device.Id]
		if !exists {
			//. Config did not exist,using default
			config = DeviceConfig{
				Name:         device.Name,
				IsShown:      true,
				OriginalName: device.Name,
			}
		}

		//* Create new name entry
		newNameEntry := &widget.Entry{
			PlaceHolder: "Device Name",
			Text:        config.Name,
		}

		//* Create reset button
		resetButton := fyneCustom.NewColorButton("", color.RGBA{68, 72, 81, 255}, theme.MediaReplayIcon(), func() {
			newNameEntry.SetText(config.OriginalName)
		})

		newNameEntry.ActionItem = resetButton

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
				Name:         newNameEntry.Text,
				IsShown:      showHideCheckbox.Checked,
				OriginalName: audioDevices[i].Name,
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
		configWin.Hide()
		configWindowOpen = false
		configButton.Enable()
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

func slicesEqual(a, b interface{}) bool {
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if va.Kind() != reflect.Slice || vb.Kind() != reflect.Slice {
		return false // Ensures the provided interfaces are slices.
	}
	if va.Len() != vb.Len() {
		return false // Slices of different lengths are not equal.
	}
	for i := 0; i < va.Len(); i++ {
		if !reflect.DeepEqual(va.Index(i).Interface(), vb.Index(i).Interface()) {
			return false // Uses deep comparison for each element.
		}
	}
	return true
}
