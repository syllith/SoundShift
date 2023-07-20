package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"soundshift/file"
	"soundshift/fyneTheme"
	"soundshift/general"
	"soundshift/interfaces/mmDeviceEnumerator"
	"soundshift/interfaces/policyConfig"
	"soundshift/winapi"
	"time"

	"github.com/energye/systray"
	"github.com/lxn/win"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
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

type ColorButton struct {
	widget.Button
	backgroundColor color.Color
	icon            fyne.Resource
}

func NewColorButton(label string, bgColor color.Color, icon fyne.Resource, tapped func()) *ColorButton {
	btn := &ColorButton{
		Button:          *widget.NewButtonWithIcon(label, icon, tapped),
		backgroundColor: bgColor,
		icon:            icon,
	}
	btn.ExtendBaseWidget(btn)
	return btn
}

func (c *ColorButton) CreateRenderer() fyne.WidgetRenderer {
	return &colorButtonRenderer{
		button:       c,
		textRenderer: canvas.NewText(c.Text, color.Black),
		iconRenderer: canvas.NewImageFromResource(c.icon),
		bgRenderer: &canvas.Rectangle{
			FillColor:   c.backgroundColor,
			StrokeColor: c.backgroundColor,
		},
	}
}

type colorButtonRenderer struct {
	button       *ColorButton
	textRenderer *canvas.Text
	iconRenderer *canvas.Image
	bgRenderer   *canvas.Rectangle
}

func (r *colorButtonRenderer) Destroy() {}

func (r *colorButtonRenderer) Layout(size fyne.Size) {
	r.textRenderer.Resize(size)
	r.iconRenderer.Resize(fyne.NewSize(size.Width-4, size.Height-4))
	r.bgRenderer.Resize(size)
}

func (r *colorButtonRenderer) MinSize() fyne.Size {
	textMinSize := r.textRenderer.MinSize()
	return fyne.NewSize(textMinSize.Width-4, textMinSize.Height-4)
}

func (r *colorButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bgRenderer, r.textRenderer, r.iconRenderer}
}

func (r *colorButtonRenderer) Refresh() {
	r.textRenderer.Refresh()
	r.iconRenderer.Refresh()
	r.bgRenderer.Refresh()
}

func (r *colorButtonRenderer) BackgroundColor() color.Color {
	return r.button.backgroundColor
}

//go:embed speaker.ico
var icon []byte
var title = "SoundShift"
var App fyne.App = app.NewWithID(title)
var Win fyne.Window = App.NewWindow(title)
var configWin fyne.Window = App.NewWindow("Configure")
var vbox = container.NewVBox()
var mainView = container.NewCenter(container.NewPadded(vbox))
var visible = true
var configWindowOpen = false

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
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			renderButtons()
		}
	}()
	Win.ShowAndRun()
}

func renderButtons() {
	audioDevices := mmDeviceEnumerator.GetDevices()

	// Create audio device buttons
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
			deviceName := config.Name
			if len(deviceName) > 30 {
				deviceName = general.EllipticalTruncate(deviceName, 30)
			}
			vbox.Add(widget.NewButtonWithIcon(deviceName, theme.VolumeUpIcon(), func() {
				policyConfig.SetDefaultEndPoint(audioDevices[index].Id)
				renderButtons()
				if settings.HideAfterSelection {
					winapi.HideWindow(title)
				}
			}))
		} else {
			//* Not default audio device
			deviceName := config.Name
			if len(deviceName) > 30 {
				deviceName = general.EllipticalTruncate(deviceName, 30)
			}
			vbox.Add(&widget.Button{
				Text: deviceName,
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
			configWindowOpen = true

			if configWin != nil {
				configWin.Close()
			}

			configWin = App.NewWindow("Configure")
			configWin.SetContent(genConfigForm())
			configWin.SetOnClosed(func() {
				configureButton.Enable()
				configWindowOpen = false
			})
			configWin.CenterOnScreen()
			configWin.Show()
		},
	}

	if configWindowOpen {
		configureButton.Disable()             // if config window is open, disable the button
		configWin.SetContent(genConfigForm()) // regenerate config form if window is open
	}

	vbox.Add(configureButton)

	//* Create hide button
	vbox.Add(&widget.Button{
		Text: "Hide",
		OnTapped: func() {
			winapi.HideWindow(title)
			visible = false
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
	_, err := os.Stat("settings.json")
	if os.IsNotExist(err) {
		// Default settings.
		settings = AppSettings{
			HideAfterSelection: false,
			DeviceNames:        make(map[string]DeviceConfig),
		}
		saveSettings() // this will create settings.json with default settings
		return
	}

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
		newNameEntry := &widget.Entry{}
		showHideCheckbox := widget.NewCheck("Shown                                            ", nil)

		config, exists := settings.DeviceNames[audioDevices[i].Id]
		if !exists {
			config = DeviceConfig{
				Name:    audioDevices[i].Name,
				IsShown: true,
			}
		}
		newNameEntry.SetText(config.Name)
		showHideCheckbox.SetChecked(config.IsShown)

		// Add an action button to newNameEntry
		newNameEntry.ActionItem = NewColorButton("", color.RGBA{68, 72, 81, 255}, theme.MediaReplayIcon(), func() {
			newNameEntry.SetText(deviceName) // reset name
		})

		form.Append(deviceName, newNameEntry)
		form.Append("", showHideCheckbox)
	}

	hideAfterSelectionCheckbox := widget.NewCheck("Hide after selection", nil)
	hideAfterSelectionCheckbox.SetChecked(settings.HideAfterSelection)

	startWithWindowsCheckbox := widget.NewCheck("Start with Windows", nil)
	startWithWindowsCheckbox.SetChecked(file.Exists(file.RoamingDir() + "/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk"))

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
				file.Delete(file.RoamingDir() + "/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk")
			}
		}

		saveSettings()
		renderButtons()
		configWin.Close()
	})

	saveButtonContainer := container.New(layout.NewCenterLayout(), saveButton)
	checkboxAndButtonVBox := container.NewVBox(hideAfterSelectionCheckbox, startWithWindowsCheckbox, saveButtonContainer)
	centeredCheckboxAndButtonContainer := container.New(layout.NewCenterLayout(), checkboxAndButtonVBox)
	borderContainer := container.NewBorder(form, centeredCheckboxAndButtonContainer, nil, nil, nil)
	return borderContainer
}

func initTray() {
	systray.SetIcon(icon)
	systray.SetTitle(title)
	systray.SetTooltip(title)
	systray.SetOnClick(func() {
		if visible {
			winapi.HideWindow(title)
			visible = false
		} else {
			winapi.ShowWindow(title)
			visible = true
		}
	})

	mQuit := systray.AddMenuItem("Exit", "Completely exit SoundShift")
	mQuit.Enable()
	mQuit.Click(func() {
		os.Exit(0)
	})
}
