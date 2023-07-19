package main

import (
	_ "embed"
	"encoding/json"
	"io/ioutil"
	"log"
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
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed speaker.ico
var iconImg []byte

var Version string = "1.00"
var App fyne.App = app.NewWithID("SoundShift")
var Win fyne.Window = App.NewWindow("SoundShift")
var mainView = container.NewCenter(container.NewPadded(vbox))
var vbox = container.NewVBox()
var configureWindow fyne.Window

type DeviceConfig struct {
	Name    string
	IsShown bool
}

var deviceNames map[string]DeviceConfig = make(map[string]DeviceConfig)

var screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
var screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
var taskbarHeight = winapi.GetTaskbarHeight()

func main() {
	Win.Resize(fyne.NewSize(250, 300))
	Win.SetTitle("SoundShift")
	Win.SetContent(mainView)
	Win.SetFixedSize(true)
	Win.SetMaster()
	Win.SetIcon(fyne.NewStaticResource("icon", iconImg))
	// configureWindow.SetIcon(fyne.NewStaticResource("icon", iconImg))
	App.Settings().SetTheme(fyneTheme.CustomTheme{})
	go systray.Run(onReady, func() {})
	go func() {
		for {
			time.Sleep(250 * time.Millisecond)
			winapi.DisableMinMaxButtons("SoundShift")
			winapi.DisableCloseButton("SoundShift")

			size := Win.Canvas().Size()
			winapi.MoveWindow("SoundShift", int32(screenWidth-int(size.Width)-20), int32(screenHeight-int(size.Height)-45-taskbarHeight), int32(size.Width), int32(size.Height))
		}
	}()

	loadDeviceNames()
	renderButtons()
	Win.ShowAndRun()
}

func loadDeviceNames() {
	file, err := ioutil.ReadFile("device_names.json")
	if err != nil {
		log.Printf("Unable to read the device names file: %v", err)
		return
	}

	err = json.Unmarshal(file, &deviceNames)
	if err != nil {
		log.Printf("Unable to unmarshal the device names file: %v", err)
	}
}

func saveDeviceNames() {
	file, err := json.Marshal(deviceNames)
	if err != nil {
		log.Printf("Unable to marshal the device names: %v", err)
		return
	}

	err = ioutil.WriteFile("device_names.json", file, 0644)
	if err != nil {
		log.Printf("Unable to write the device names to a file: %v", err)
	}
}

func renderButtons() {
	vbox.Objects = nil

	audioDevices := mmDeviceEnumerator.GetDevices()

	for i := 0; i < len(audioDevices); i++ {
		index := i

		// The name of the device now comes from the map we created.
		// If there's no new name for the device in the map, then we use the default name.
		config, exists := deviceNames[audioDevices[i].Id]
		if !exists {
			config = DeviceConfig{
				Name:    audioDevices[i].Name,
				IsShown: true,
			}
		}

		if !config.IsShown {
			continue // Don't render the button if the device is not shown.
		}

		if audioDevices[i].IsDefault {
			vbox.Add(widget.NewButtonWithIcon(config.Name, theme.VolumeUpIcon(), func() {
				policyConfig.SetDefaultEndPoint(audioDevices[index].Id)
				renderButtons()
			}))
		} else {
			vbox.Add(&widget.Button{
				Text: config.Name,
				OnTapped: func() {
					policyConfig.SetDefaultEndPoint(audioDevices[index].Id)
					renderButtons()
				},
			})
		}
	}

	vbox.Add(&widget.Label{
		Text: "                                       ",
	})

	vbox.Add(&widget.Button{
		Text: "Configure",
		OnTapped: func() {
			// Disable the button if the configuration window is already open
			if configureWindow != nil {
				return
			}
			btn := vbox.Objects[len(vbox.Objects)-3].(*widget.Button) // Assuming the "Configure" button is always the third last in the vbox
			btn.Disable()

			// Create new fyne window if it doesn't exist
			configureWindow = App.NewWindow("Configure")
			configureWindow.SetContent(generateConfigureForm())
			configureWindow.SetOnClosed(func() {
				configureWindow = nil
				// Enable the button when the configuration window is closed
				btn.Enable()
			})
			configureWindow.CenterOnScreen()
			configureWindow.Show()
		},
	})

	vbox.Add(&widget.Button{
		Text: "Hide",
		OnTapped: func() {
			Win.Hide()
		},
	})

	mainView.Refresh()
}

func onReady() {
	systray.SetIcon(iconImg)
	systray.SetTitle("SoundShift")
	systray.SetTooltip("SoundShift")
	systray.SetOnClick(func() {
		Win.Show()
	})

	mQuit := systray.AddMenuItem("Exit", "Completely exit SoundShift")
	mQuit.Enable()
	mQuit.Click(func() {
		os.Exit(0)
	})
}

func generateConfigureForm() fyne.CanvasObject {
	form := &widget.Form{}

	audioDevices := mmDeviceEnumerator.GetDevices()

	for i := 0; i < len(audioDevices); i++ {
		deviceName := audioDevices[i].Name
		newNameEntry := widget.NewEntry()
		showHideCheckbox := widget.NewCheck("Shown                      ", nil)

		// Load the device name from the map.
		config, exists := deviceNames[audioDevices[i].Id]
		if !exists {
			config = DeviceConfig{
				Name:    audioDevices[i].Name,
				IsShown: true,
			}
		}
		newNameEntry.SetText(config.Name)
		showHideCheckbox.SetChecked(config.IsShown)

		// Add the device name, entry, and checkbox container to the form
		form.Append(deviceName, newNameEntry)
		form.Append("", showHideCheckbox)
	}

	saveButton := widget.NewButton("     Save     ", func() {
		// Loop through all of the form items and save the device name entered by the user.
		for i := 0; i < len(audioDevices); i++ {
			newNameEntry := form.Items[i*2].Widget.(*widget.Entry)
			showHideCheckbox := form.Items[i*2+1].Widget.(*widget.Check)

			// Update the device names map
			deviceNames[audioDevices[i].Id] = DeviceConfig{
				Name:    newNameEntry.Text,
				IsShown: showHideCheckbox.Checked,
			}
		}

		// Save the device names to a file.
		saveDeviceNames()

		// Re-render the buttons
		renderButtons()
	})

	// Create a centered container for the save button
	saveButtonContainer := container.New(layout.NewCenterLayout(), saveButton)

	// Create a border container to hold the form and the save button container
	borderContainer := container.NewBorder(form, saveButtonContainer, nil, nil, nil)

	return borderContainer
}
