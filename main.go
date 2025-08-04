package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image/color"
	"os"
	"runtime"
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
	"golang.org/x/sys/windows"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// . Structs for application configuration and device settings
type DeviceConfig struct {
	Name         string
	IsShown      bool
	OriginalName string
}

type AppSettings struct {
	HideAfterSelection bool
	DeviceNames        map[string]DeviceConfig
}

// . Global variables for application state and configuration
var configWindowOpen = false
var settings AppSettings
var currentDeviceID string
var audioDevices []mmDeviceEnumerator.AudioDevice
var screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
var screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
var taskbarHeight = winapi.GetTaskbarHeight()
var hwnd windows.HWND
var deviceVboxPlaceholder = container.New(&fyneCustom.CustomVBoxLayout{FixedWidth: 150})

// . Diagnostic logging functions
func logSession(tag string) {
	var sid uint32
	windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sid)
	general.LogError(fmt.Sprintf("%s (session=%d)", tag, sid), nil)
}

// . waitForHWNDByTitle waits for a valid window handle with timeout
func waitForHWNDByTitle(pid uint32, title string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h, err := winapi.GetHwnd(pid, title)
		if err == nil && h != 0 {
			hwnd = h
			general.LogError(fmt.Sprintf("HWND_ACQUIRED (hwnd=%d)", uintptr(h)), nil)
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout acquiring hwnd")
}

// . toggleWindow toggles the main window visibility
func toggleWindow() {
	if hwnd == 0 {
		general.LogError("toggle with hwnd==0", fmt.Errorf("no hwnd"))
		return
	}
	if winapi.IsWindowVisible(hwnd) {
		general.LogError("TOGGLE_HIDE", nil)
		winapi.HideWindow(hwnd)
	} else {
		general.LogError("TOGGLE_SHOW", nil)
		fyne.Do(func() {
			refreshScreenMetrics()
			resize()
			winapi.ShowWindow(hwnd)
			winapi.SetTopmost(hwnd)
		})
	}
}

// . cleanExit performs proper cleanup before exiting
func cleanExit() {
	general.LogError("CLEAN_EXIT", nil)
	mouse.Uninstall()
	systray.Quit()
	App.Quit()
}

//go:embed speaker.ico
var icon []byte
var title = "SoundShift"
var App fyne.App = app.New()
var Win fyne.Window = App.NewWindow(title)
var configWin fyne.Window = App.NewWindow("Configure")

// * Main view layout
var mainView = container.NewPadded(
	container.NewCenter(
		container.NewVBox(
			deviceVboxPlaceholder,
			&canvas.Line{StrokeColor: colormap.Gray, StrokeWidth: 1},
			configButton,
			container.NewPadded(volumeSlider),
		),
	),
)

// * Configure button to open settings window
var configButton = &widget.Button{Text: "Configure"}

// * Scrollable volume slider control
var volumeSlider = fyneCustom.NewScrollableSlider(0, 100)

// . Initialization function for setting up UI interactions
func init() {
	//* Set up volume slider callback to adjust volume when changed
	volumeSlider.OnChanged = func(f float64) {
		volumeScalar := float32(f / 100.0)
		if currentDeviceID != "" {
			// Skip setting volume for disabled slider (inaccessible Remote Audio)
			if volumeSlider.Disabled {
				return
			}

			// Check if this is Remote Audio and test accessibility before setting volume
			for _, device := range audioDevices {
				if device.Id == currentDeviceID && device.Name == "Remote Audio" {
					// Test if Remote Audio is accessible before attempting to set volume
					_, err := policyConfig.GetVolume(currentDeviceID)
					if err != nil {
						// Remote Audio is not accessible, don't try to set volume
						return
					}
					break
				}
			}

			if err := policyConfig.SetVolume(currentDeviceID, volumeScalar); err != nil {
				fmt.Println("Error setting volume:", err)
				general.LogError("Error setting volume:", err)
			}
		}
	}

	//* Configure the behavior of the config window and button
	configWin = fyne.CurrentApp().NewWindow("Configure")
	configButton.OnTapped = func() {
		//* Open the config window and disable the button to prevent multiple instances
		if !configWindowOpen {
			configWin.Show()
			configWin.CenterOnScreen()
			configWindowOpen = true
			configButton.Disable()
		}
	}

	//* Handle config window close event to reset button state
	configWin.SetCloseIntercept(func() {
		configWin.Hide()
		configWindowOpen = false
		configButton.Enable()
	})
}

func refreshScreenMetrics() {
	screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
	screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
	taskbarHeight = winapi.GetTaskbarHeight()
}

func resize() {
	if hwnd == 0 {
		return
	}
	refreshScreenMetrics() // <-- now recalculated each call

	size := Win.Content().MinSize()
	paddingX := int(float64(screenWidth) * 0.02)
	paddingY := int(float64(screenHeight) * 0.05)

	winapi.MoveWindow(
		hwnd,
		int32(screenWidth-int(size.Width)-paddingX),
		int32(screenHeight-int(size.Height)-paddingY-taskbarHeight),
		int32(size.Width),
		int32(size.Height),
	)
}

// . main initializes the application, sets up the UI and systray, and manages application lifecycle events.
func main() {
	//* Log application start
	logSession("APP_START")

	//* Exit if an instance of the application is already running
	if general.IsProcRunning(title) {
		os.Exit(0)
	}

	//* Initialize COM library for interacting with Windows APIs
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		fmt.Println("Failed to initialize COM library:", err)
		return
	}
	defer ole.CoUninitialize()

	//* Load saved settings for audio devices
	loadSettings()

	//* Retrieve the current process ID for identifying application windows
	pid := windows.GetCurrentProcessId()

	//* Configure application theme
	App.Settings().SetTheme(fyneTheme.CustomTheme{})

	//* Configure main window properties and layout
	Win.SetContent(mainView)
	Win.SetTitle(title)
	Win.SetIcon(fyne.NewStaticResource("icon", icon))
	Win.SetCloseIntercept(func() {
		//* Intercept window close to hide it instead of terminating the app
		winapi.HideWindow(hwnd)
	})

	//* Configure config window properties and layout
	configWin.SetIcon(fyne.NewStaticResource("icon", icon))
	configWin.SetContent(genConfigForm())
	configWin.Resize(fyne.NewSize(600, 500))
	configWin.SetOnClosed(func() {
		//* Re-enable config button when the config window is closed
		configButton.Enable()
		configWindowOpen = false
	})

	//* Show the main window initially to get the window handle
	Win.Show()

	//* Wait for a valid window handle with timeout
	if err := waitForHWNDByTitle(pid, title, 5*time.Second); err != nil {
		general.LogError("Failed to acquire HWND", err)
		return
	}

	//* Apply Windows API settings to the application window
	resize()
	winapi.HideWindow(hwnd)
	winapi.HideMinMaxButtons(hwnd)
	winapi.HideWindowFromTaskbar(hwnd)
	winapi.SetTopmost(hwnd)

	//* Start systray on a locked OS thread
	go func() {
		runtime.LockOSThread()
		systray.Run(initTray, func() {})
	}()

	//* Start background goroutines for handling clicks and device updates
	go hideOnClick()
	go updateDevices()
	go monitorDeviceChanges()

	//* Run the application event loop
	Win.ShowAndRun()
}

// . checkAndUpdateDevices checks for changes in the list of audio devices and updates the UI if changes are detected
func checkAndUpdateDevices() {
	//* Retrieve the current list of audio devices
	newAudioDevices, err := mmDeviceEnumerator.GetDevices()
	if err != nil || newAudioDevices == nil {
		//! Log error if device retrieval fails
		fmt.Println("Error getting audio devices:", err)
		general.LogError("Error getting audio devices:", err)
		return
	}

	// Filter out inaccessible devices, but always include Remote Audio
	validDevices := make([]mmDeviceEnumerator.AudioDevice, 0, len(newAudioDevices))
	var remoteAudio *mmDeviceEnumerator.AudioDevice = nil
	for _, device := range newAudioDevices {
		if device.Name == "Remote Audio" {
			copy := device
			remoteAudio = &copy
			continue
		}
		_, err := policyConfig.GetVolume(device.Id)
		if err == nil {
			validDevices = append(validDevices, device)
		}
		// Don't log here since this runs every 3 seconds and would be too noisy
	}
	if remoteAudio != nil {
		validDevices = append(validDevices, *remoteAudio)
	}

	//* Check if the list of devices has changed and config window is closed
	if !audioDevicesEqual(audioDevices, validDevices) && !configWindowOpen {
		//* Update audio devices if changes are detected
		audioDevices = validDevices

		//* Reset currentDeviceID to prevent stale device references
		currentDeviceID = ""

		loadSettings() // Reload settings to handle device ID changes
		fyne.Do(func() {
			renderButtons()
			resize()
		})
	}
}

// . updateDevices continuously checks for audio device changes at regular intervals
func updateDevices() {
	// Force an immediate fresh device list retrieval on startup
	freshDevices, err := mmDeviceEnumerator.GetDevices()
	if err == nil && freshDevices != nil {
		// Filter out any devices that can't be accessed, but always include Remote Audio
		validDevices := make([]mmDeviceEnumerator.AudioDevice, 0, len(freshDevices))
		var remoteAudio *mmDeviceEnumerator.AudioDevice = nil
		for _, device := range freshDevices {
			if device.Name == "Remote Audio" {
				copy := device
				remoteAudio = &copy
				continue
			}
			_, err := policyConfig.GetVolume(device.Id)
			if err == nil {
				validDevices = append(validDevices, device)
			} else {
				fmt.Printf("Skipping inaccessible device %s (%s): %v\n", device.Name, device.Id, err)
			}
		}
		if remoteAudio != nil {
			validDevices = append(validDevices, *remoteAudio)
		}

		audioDevices = validDevices
		fyne.Do(func() {
			renderButtons()
			resize()
		})
	}

	// Set up the ticker for subsequent device updates
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	// Periodically check for audio device updates
	for range ticker.C {
		checkAndUpdateDevices() // Check and update device list every 3 seconds
	}
}

// . renderButtons dynamically creates and updates buttons for each audio device in the UI
func renderButtons() {
	//* Create a new container for device buttons
	newDeviceVbox := container.New(&fyneCustom.CustomVBoxLayout{FixedWidth: 150})

	//* Create a button for each audio device
	for _, device := range audioDevices {
		//* Retrieve device configuration (use defaults if config does not exist)
		config, exists := settings.DeviceNames[device.Id]
		if !exists {
			config = DeviceConfig{Name: device.Name, IsShown: true}
		}

		//* Skip device if it is marked as hidden
		if !config.IsShown {
			continue
		}

		//* Get a truncated version of the device name for display
		deviceName := general.EllipticalTruncate(config.Name, 15)

		//* Create the button tap handler for selecting a device
		onTapped := createDeviceButtonHandler(device.Id)

		//* Add button for the device to newDeviceVbox
		if device.IsDefault {
			if device.Name == "Remote Audio" {
				newDeviceVbox.Add(widget.NewButtonWithIcon(deviceName+" (RDP)", theme.VolumeUpIcon(), func() {
					fyne.CurrentApp().SendNotification(&fyne.Notification{
						Title:   "Remote Audio",
						Content: "Remote Audio device may not support volume/mute control over RDP.",
					})
					onTapped()
				}))
				// Try to get volume, but don't error if it fails
				volume, err := policyConfig.GetVolume(device.Id)
				if err == nil {
					currentDeviceID = device.Id
					volumeSlider.Disabled = false
					volumeSlider.Enable()
					volumeSlider.SetValue(float64(volume * 100))
					muted, err := policyConfig.GetMute(currentDeviceID)
					if err == nil && muted {
						volumeSlider.SetValue(0)
					}
				} else {
					currentDeviceID = device.Id // Still set so user can try
					volumeSlider.Disabled = true
					volumeSlider.Disable()
					volumeSlider.SetValue(100)
				}
			} else {
				newDeviceVbox.Add(widget.NewButtonWithIcon(deviceName, theme.VolumeUpIcon(), onTapped))
				volume, err := policyConfig.GetVolume(device.Id)
				if err != nil {
					fmt.Printf("Error getting volume for device %s: %v\n", device.Id, err)
				} else {
					currentDeviceID = device.Id
					volumeSlider.Enable()
					volumeSlider.SetValue(float64(volume * 100))
					muted, err := policyConfig.GetMute(currentDeviceID)
					if err != nil {
						fmt.Printf("Error getting mute state for device %s: %v\n", currentDeviceID, err)
					} else if muted {
						volumeSlider.SetValue(0)
					}
				}
			}
		} else {
			if device.Name == "Remote Audio" {
				newDeviceVbox.Add(widget.NewButton(deviceName+" (RDP)", func() {
					fyne.CurrentApp().SendNotification(&fyne.Notification{
						Title:   "Remote Audio",
						Content: "Remote Audio device may not support volume/mute control over RDP.",
					})
					onTapped()
				}))
				// Always set slider to 100 and disable for non-default Remote Audio
				if currentDeviceID == device.Id {
					volumeSlider.Disabled = true
					volumeSlider.Disable()
					volumeSlider.SetValue(100)
				}
			} else {
				newDeviceVbox.Add(widget.NewButton(deviceName, onTapped))
			}
		}
	}

	//* Refresh the container only once after adding all buttons
	newDeviceVbox.Refresh()

	//* Replace the old deviceVbox with the new one
	deviceVboxPlaceholder.Objects = []fyne.CanvasObject{newDeviceVbox}
	deviceVboxPlaceholder.Refresh()
}

// . loadSettings loads application settings from a JSON file, initializing defaults if the file doesn't exist
func loadSettings() {
	settingsPath := file.RoamingDir() + "/soundshift/settings.json"

	//* Initialize settings with default values
	settings = AppSettings{
		HideAfterSelection: false,
		DeviceNames:        make(map[string]DeviceConfig),
	}

	//* Attempt to read the settings file from disk
	fileData, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			//* Settings file does not exist; create a new one with default settings
			saveSettings()
		}
		return // Exit if there's an error reading the file
	}

	//* Parse settings from JSON file data
	json.Unmarshal(fileData, &settings)

	//* Ensure DeviceNames map is initialized even if missing from file
	if settings.DeviceNames == nil {
		settings.DeviceNames = make(map[string]DeviceConfig)
	}

	//* Build a map linking original device names to their IDs for device ID consistency
	originalNameToConfig := make(map[string]string) // Maps OriginalName to Device ID
	for id, config := range settings.DeviceNames {
		originalNameToConfig[config.OriginalName] = id
	}

	//* Retrieve the list of currently connected audio devices
	currentDevices, err := mmDeviceEnumerator.GetDevices()
	if err != nil {
		fmt.Println("Error getting current devices:", err)
		general.LogError("Error getting current devices:", err)
		return
	}

	//* Temporary map to store updated device configurations
	updatedDeviceNames := make(map[string]DeviceConfig)

	//* Update settings for each detected device, preserving configurations when possible
	for _, device := range currentDevices {
		config, exists := settings.DeviceNames[device.Id]
		if !exists {
			//* Check if there’s an existing config with the same OriginalName but different ID
			if oldID, found := originalNameToConfig[device.Name]; found {
				//* Reassign the config to the new device ID
				config = settings.DeviceNames[oldID]
				updatedDeviceNames[device.Id] = config
				delete(settings.DeviceNames, oldID) // Remove old ID entry to prevent duplication
				fmt.Printf("Updated device ID for %s from %s to %s\n", device.Name, oldID, device.Id)
			} else {
				//* No existing config for this device; create a default config
				config = DeviceConfig{
					Name:         device.Name,
					IsShown:      true,
					OriginalName: device.Name,
				}
				updatedDeviceNames[device.Id] = config
			}
		} else {
			//* Device ID exists in settings, so reuse the existing config
			updatedDeviceNames[device.Id] = config
		}
	}

	//* Update settings with the revised device configurations
	settings.DeviceNames = updatedDeviceNames
}

// . saveSettings saves the current settings to a JSON file in the user's roaming directory
func saveSettings() {
	fileData, _ := json.MarshalIndent(settings, "", "    ")

	//* Ensure the settings directory exists
	os.MkdirAll(file.RoamingDir()+"/soundshift", os.ModePerm)

	//* Write the settings file to disk
	os.WriteFile(file.RoamingDir()+"/soundshift/settings.json", fileData, 0644)
}

// . genConfigForm generates a configuration form for managing audio device settings and application options
func genConfigForm() fyne.CanvasObject {
	//* Retrieve the current list of audio devices
	audioDevices, err := mmDeviceEnumerator.GetDevices()
	if err != nil || audioDevices == nil {
		fmt.Println("Error getting audio devices:", err)
		general.LogError("Error getting audio devices:", err)
		return nil
	}

	//* Initialize a new form for device settings
	form := &widget.Form{}
	for _, device := range audioDevices {
		//* Retrieve or initialize device configuration
		config, exists := settings.DeviceNames[device.Id]
		if !exists {
			// . Config does not exist; initialize with default values
			config = DeviceConfig{
				Name:         device.Name,
				IsShown:      true,
				OriginalName: device.Name,
			}
		}

		//* Create an entry field for renaming the device
		newNameEntry := &widget.Entry{
			PlaceHolder: "Device Name",
			Text:        config.Name,
		}

		//* Create a reset button to revert the name to the original device name
		resetButton := fyneCustom.NewColorButton("", color.RGBA{68, 72, 81, 255}, theme.MediaReplayIcon(), func() {
			newNameEntry.SetText(config.OriginalName)
		})
		newNameEntry.ActionItem = resetButton

		//* Create a checkbox to show/hide the device in the main interface
		showHideCheckbox := &widget.Check{
			Text:    "Shown",
			Checked: config.IsShown,
		}

		//* Add device entry and visibility checkbox to the form
		form.Append(device.Name, newNameEntry)
		form.Append("", showHideCheckbox)
	}

	//* Checkbox for hiding the application window after selecting a device
	hideAfterSelectionCheckbox := &widget.Check{
		Text:    "Hide after selection",
		Checked: settings.HideAfterSelection,
	}

	//* Checkbox for setting the application to start with Windows
	startWithWindowsCheckbox := &widget.Check{
		Text:    "Start with Windows",
		Checked: file.Exists(file.RoamingDir() + "/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk"),
	}

	//* Save button to apply and persist settings
	saveButton := widget.NewButton("     Save     ", func() {
		for i := 0; i < len(audioDevices); i++ {
			//* Update settings based on form inputs
			newNameEntry := form.Items[i*2].Widget.(*widget.Entry)
			showHideCheckbox := form.Items[i*2+1].Widget.(*widget.Check)

			settings.DeviceNames[audioDevices[i].Id] = DeviceConfig{
				Name:         newNameEntry.Text,
				IsShown:      showHideCheckbox.Checked,
				OriginalName: audioDevices[i].Name,
			}
		}

		//* Apply global settings based on checkbox states
		settings.HideAfterSelection = hideAfterSelectionCheckbox.Checked

		//* Manage application startup with Windows based on checkbox state
		startupPath := file.RoamingDir() + "/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk"
		if startWithWindowsCheckbox.Checked {
			//* Create a startup shortcut if it doesn't exist
			if !file.Exists(startupPath) {
				general.CreateShortcut(file.Cwd()+"/soundshift.exe", startupPath)
			}
		} else {
			//* Remove the startup shortcut if it exists
			if file.Exists(startupPath) {
				os.Remove(startupPath)
			}
		}

		//* Update hide-after-selection setting
		settings.HideAfterSelection = hideAfterSelectionCheckbox.Checked

		//* Save settings to file and close the configuration window
		saveSettings()
		configWin.Hide()
		configWindowOpen = false
		configButton.Enable()

		//* Refresh the main UI to reflect updated settings
		renderButtons()
		time.Sleep(100 * time.Millisecond)
		resize()
	})

	//* Layout for save button and checkboxes
	saveButtonContainer := container.New(layout.NewCenterLayout(), saveButton)
	checkboxAndButtonVBox := container.NewVBox(hideAfterSelectionCheckbox, startWithWindowsCheckbox, saveButtonContainer)
	centeredCheckboxAndButtonContainer := container.New(layout.NewCenterLayout(), checkboxAndButtonVBox)

	//* Create a padded border container for the form and additional settings
	borderContainer := container.NewPadded(container.NewBorder(form, centeredCheckboxAndButtonContainer, nil, nil, nil))
	return borderContainer
}

// . initTray initializes the system tray icon and menu, allowing for window toggling and application exit
func initTray() {
	logSession("TRAY_INIT")

	//* Set the tray icon, title, and tooltip
	systray.SetIcon(icon)
	systray.SetTitle(title)
	systray.SetTooltip(title)

	//* Handle left-click on the tray icon to toggle window visibility
	systray.SetOnClick(func(menu systray.IMenu) {
		general.LogError("TRAY_ICON_CLICKED", nil)
		toggleWindow()
	})

	//* Create menu items for right-click context menu
	mToggle := systray.AddMenuItem("Show / Hide", "Toggle window")
	mToggle.Click(func() {
		general.LogError("TRAY_TOGGLE_CLICKED", nil)
		toggleWindow()
	})

	//* Add a "Quit" option to the tray menu to allow the user to exit the application
	mQuit := systray.AddMenuItem("Exit", "Completely exit SoundShift")
	mQuit.Enable()
	mQuit.Click(func() {
		general.LogError("TRAY_EXIT_CLICKED", nil)
		cleanExit()
	})
}

// . hideOnClick hides the application window if a mouse click occurs outside of it, with debouncing
func hideOnClick() {
	mouseChan := make(chan types.MouseEvent, 1024) // Buffered channel to prevent blocking
	mouse.Install(nil, mouseChan)
	defer mouse.Uninstall()

	//* Monitor mouse events for click actions
	for k := range mouseChan {
		if k.Message == 513 { // WM_LBUTTONDOWN
			//* Check if the click is outside the application window and taskbar
			if !isMouseInWindow() && !isMouseInTaskbar() && !configWindowOpen {
				winapi.HideWindow(hwnd)
			}
		}
	}
}

// . isMouseInWindow checks if the mouse cursor is currently within the application window boundaries
func isMouseInWindow() bool {
	//* Get current mouse coordinates
	xMouse, yMouse := robotgo.Location()
	//* Get window position and size
	xPos, yPos, _ := winapi.GetWindowPosition(hwnd)
	xSize, ySize, _ := winapi.GetWindowSize(hwnd)

	//* Check if mouse coordinates fall within window boundaries
	return int(xMouse) >= int(xPos) && int(xMouse) <= int(xPos+xSize) &&
		int(yMouse) >= int(yPos) && int(yMouse) <= int(yPos+ySize)
}

// . isMouseInTaskbar checks if the mouse cursor is currently within the taskbar area
func isMouseInTaskbar() bool {
	_, yMouse := robotgo.Location() // Get the Y coordinate of the mouse cursor
	//* Get fresh screen height to handle display changes (remote desktop, monitor changes, etc.)
	currentScreenHeight := int(win.GetSystemMetrics(win.SM_CYSCREEN))
	//* Check if the mouse Y coordinate is within taskbar height from the bottom of the screen
	return currentScreenHeight-yMouse <= winapi.GetTaskbarHeight()
}

// . audioDevicesEqual performs a fast comparison of AudioDevice slices without reflection
func audioDevicesEqual(a, b []mmDeviceEnumerator.AudioDevice) bool {
	// Quick length check first
	if len(a) != len(b) {
		return false
	}

	// Compare each device's properties directly
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Id != b[i].Id ||
			a[i].IsDefault != b[i].IsDefault {
			return false
		}
	}
	return true
}

func monitorDeviceChanges() {
	ticker := time.NewTicker(500 * time.Millisecond) // Poll every 500 ms
	defer ticker.Stop()

	// Cache previous values to avoid unnecessary UI updates
	var lastVolume float32 = -1
	var lastMuted bool = false
	var lastDeviceID string = ""
	var isRemoteAudioInaccessible bool = false

	for {
		<-ticker.C

		// Skip monitoring if window is not visible
		if !winapi.IsWindowVisible(hwnd) {
			continue
		}

		// Skip if no device is selected
		if currentDeviceID == "" {
			continue
		}

		// Reset cache if device changed
		if currentDeviceID != lastDeviceID {
			lastVolume = -1
			lastMuted = false
			lastDeviceID = currentDeviceID
			isRemoteAudioInaccessible = false
		}

		// Validate that the current device still exists in the device list
		deviceExists := false
		var currentDevice mmDeviceEnumerator.AudioDevice
		for _, device := range audioDevices {
			if device.Id == currentDeviceID {
				deviceExists = true
				currentDevice = device
				break
			}
		}

		// If device no longer exists, reset currentDeviceID and skip monitoring
		if !deviceExists {
			fmt.Printf("Device %s no longer exists, resetting current device\n", currentDeviceID)
			currentDeviceID = ""
			lastDeviceID = ""
			continue
		}

		// Skip volume/mute polling for inaccessible Remote Audio devices
		if currentDevice.Name == "Remote Audio" && !isRemoteAudioInaccessible {
			// Test if Remote Audio is accessible on first check
			_, err := policyConfig.GetVolume(currentDeviceID)
			if err != nil {
				isRemoteAudioInaccessible = true
				// Set slider to disabled state and continue without further polling
				fyne.Do(func() {
					volumeSlider.Disabled = true
					volumeSlider.Disable()
					volumeSlider.SetValue(100)
				})
				continue
			}
		}

		// Skip polling for inaccessible Remote Audio
		if currentDevice.Name == "Remote Audio" && isRemoteAudioInaccessible {
			continue
		}

		// Retrieve current volume and mute state
		volume, err := policyConfig.GetVolume(currentDeviceID)
		if err != nil {
			// For Remote Audio, mark as inaccessible and continue
			if currentDevice.Name == "Remote Audio" {
				isRemoteAudioInaccessible = true
				fyne.Do(func() {
					volumeSlider.Disabled = true
					volumeSlider.Disable()
					volumeSlider.SetValue(100)
				})
				continue
			}
			fmt.Printf("Error getting volume for device %s: %v\n", currentDeviceID, err)
			// Reset currentDeviceID if there's a persistent error for non-Remote Audio devices
			currentDeviceID = ""
			lastDeviceID = ""
			continue
		}

		muted, err := policyConfig.GetMute(currentDeviceID)
		if err != nil {
			// For Remote Audio, mark as inaccessible and continue
			if currentDevice.Name == "Remote Audio" {
				isRemoteAudioInaccessible = true
				fyne.Do(func() {
					volumeSlider.Disabled = true
					volumeSlider.Disable()
					volumeSlider.SetValue(100)
				})
				continue
			}
			fmt.Printf("Error getting mute state for device %s: %v\n", currentDeviceID, err)
			// Reset currentDeviceID if there's a persistent error for non-Remote Audio devices
			currentDeviceID = ""
			lastDeviceID = ""
			continue
		}

		// Only update UI if values have actually changed
		if volume != lastVolume || muted != lastMuted {
			lastVolume = volume
			lastMuted = muted

			fyne.Do(func() {
				if muted {
					volumeSlider.SetValue(0)
				} else {
					volumeSlider.SetValue(float64(volume * 100))
				}
			})
		}
	}
}

// . createDeviceButtonHandler creates a button tap handler for device selection
func createDeviceButtonHandler(deviceID string) func() {
	return func() {
		if err := policyConfig.SetDefaultEndPoint(deviceID); err != nil {
			fmt.Println("Error setting default endpoint:", err)
			general.LogError("Error setting default endpoint:", err)
			return
		}
		// Wait for the system to update the default endpoint
		time.Sleep(200 * time.Millisecond)
		// Refresh device list and update UI accordingly
		audioDevices, _ = mmDeviceEnumerator.GetDevices()
		for i := range audioDevices {
			audioDevices[i].IsDefault = (audioDevices[i].Id == deviceID)
		}
		renderButtons()
		if settings.HideAfterSelection {
			winapi.HideWindow(hwnd)
		}
	}
}
