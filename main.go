package main

import (
	"soundshift/fyneTheme"
	"soundshift/interfaces/mmDeviceEnumerator"
	"soundshift/interfaces/policyConfig"
	"soundshift/winapi"
	"time"

	"github.com/energye/systray"
	"github.com/energye/systray/icon"
	"github.com/lxn/win"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var Version string = "1.00"
var App fyne.App = app.NewWithID("SoundShift")
var Win fyne.Window = App.NewWindow("SoundShift")
var mainView = container.NewCenter(container.NewPadded(vbox))
var vbox = container.NewVBox()
var configureWindow fyne.Window
var isConfigWindowOpen bool

var screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
var screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
var taskbarHeight = winapi.GetTaskbarHeight()

func main() {
	Win.Resize(fyne.NewSize(250, 300))
	Win.SetTitle("SoundShift")
	Win.SetContent(mainView)
	Win.SetFixedSize(true)
	Win.SetMaster()
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

	renderButtons()
	Win.ShowAndRun()
}

func renderButtons() {
	vbox.Objects = nil

	audioDevices := mmDeviceEnumerator.GetDevices()

	for i := 0; i < len(audioDevices); i++ {
		index := i
		if audioDevices[i].IsDefault {
			vbox.Add(widget.NewButtonWithIcon(audioDevices[i].Name, theme.VolumeUpIcon(), func() {
				policyConfig.SetDefaultEndPoint(audioDevices[index].Id)
				renderButtons()
			}))
		} else {
			vbox.Add(&widget.Button{
				Text: audioDevices[i].Name,
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

	vbox.Add(&widget.Button{
		Text: "Exit",
		OnTapped: func() {
			Win.Close()
		},
	})

	mainView.Refresh()
}

func onReady() {
	systray.SetIcon(icon.Data)
	systray.SetTitle("SoudShift")
	systray.SetTooltip("SoudShift")
	systray.SetOnClick(func() {
		Win.Show()
	})
}

func generateConfigureForm() fyne.CanvasObject {
	form := &widget.Form{}

	audioDevices := mmDeviceEnumerator.GetDevices()

	for i := 0; i < len(audioDevices); i++ {
		deviceName := audioDevices[i].Name
		newNameEntry := widget.NewEntry()
		showHideCheckbox := widget.NewCheck("", nil)
		hiddenLabel := widget.NewLabel("Hidden")

		// Set the current device name as the default value for the entry
		newNameEntry.SetText(deviceName)

		// Create a container to hold the checkbox and the hidden label
		checkboxContainer := container.NewHBox(showHideCheckbox, hiddenLabel)

		// Add the device name, entry, and checkbox container to the form
		form.Append(deviceName, newNameEntry)
		form.Append("", checkboxContainer)
	}

	saveButton := widget.NewButton("Save", func() {
		// Handle saving the configuration here
	})

	// Create a centered container for the save button
	saveButtonContainer := container.New(layout.NewCenterLayout(), saveButton)

	// Create a border container to hold the form and the save button container
	borderContainer := container.NewBorder(form, saveButtonContainer, nil, nil, nil)

	return borderContainer
}
