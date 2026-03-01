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
	"sync"
	"sync/atomic"
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
var configWindowOpen atomic.Bool // replaces plain bool; safe for concurrent read from goroutines
var settings AppSettings
var currentDeviceID string
var audioDevices []mmDeviceEnumerator.AudioDevice
var screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
var screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
var taskbarHeight = winapi.GetTaskbarHeight()
var hwnd windows.HWND
var deviceVboxPlaceholder = container.New(&fyneCustom.CustomVBoxLayout{FixedWidth: 150})

// . mu protects audioDevices and currentDeviceID which are accessed from multiple goroutines.
// Rule: never hold mu while calling fyne.Do.
var mu sync.Mutex

// . singleInstanceMu holds the Windows named mutex for single-instance enforcement
var singleInstanceMu windows.Handle

// . Diagnostic logging functions
func logSession(tag string) {
	var sid uint32
	windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sid)
	general.LogError(fmt.Sprintf("%s (session=%d)", tag, sid), nil)
}

// . acquireSingleInstance uses a named Windows mutex to ensure only one instance runs.
// Returns true if this process is the first (and only) instance.
func acquireSingleInstance() bool {
	name, err := windows.UTF16PtrFromString("Global\\SoundShiftSingleInstance")
	if err != nil {
		// Fallback to process-name check if UTF16 conversion fails
		return !general.IsProcRunning(title)
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		return false
	}
	if err != nil {
		// If mutex creation fails for any other reason, fall back
		return !general.IsProcRunning(title)
	}
	singleInstanceMu = handle // Keep handle open for the lifetime of the process
	return true
}

// . withRecovery runs fn in a new goroutine, logging any panic instead of crashing the process.
func withRecovery(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				general.LogError(fmt.Sprintf("panic in %s", name), fmt.Errorf("%v", r))
			}
		}()
		fn()
	}()
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
		// resize already calls refreshScreenMetrics internally — no need to call it twice
		resize()
		winapi.ShowWindow(hwnd)
		winapi.SetTopmost(hwnd)
	}
}

// . cleanExit performs proper cleanup before exiting
func cleanExit() {
	general.LogError("CLEAN_EXIT", nil)
	mouse.Uninstall()
	systray.Quit()
	fyne.Do(func() {
		App.Quit()
	})
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

		mu.Lock()
		devID := currentDeviceID
		devices := make([]mmDeviceEnumerator.AudioDevice, len(audioDevices))
		copy(devices, audioDevices)
		mu.Unlock()

		if devID == "" {
			return
		}

		// Skip setting volume for disabled slider (inaccessible Remote Audio)
		if volumeSlider.Disabled {
			return
		}

		// Check if this is Remote Audio and test accessibility before setting volume
		for _, device := range devices {
			if device.Id == devID && device.Name == "Remote Audio" {
				_, err := policyConfig.GetVolume(devID)
				if err != nil {
					return
				}
				break
			}
		}

		if err := policyConfig.SetVolume(devID, volumeScalar); err != nil {
			fmt.Println("Error setting volume:", err)
			general.LogError("Error setting volume:", err)
		}
	}

	//* Configure the behavior of the config window and button
	configWin = fyne.CurrentApp().NewWindow("Configure")
	configButton.OnTapped = func() {
		if !configWindowOpen.Load() {
			configWin.Show()
			configWin.CenterOnScreen()
			configWindowOpen.Store(true)
			configButton.Disable()
		}
	}

	//* Handle config window close event to reset button state
	configWin.SetCloseIntercept(func() {
		configWin.Hide()
		configWindowOpen.Store(false)
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
	refreshScreenMetrics()

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

	//* Exit if an instance of the application is already running (named-mutex check)
	if !acquireSingleInstance() {
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
		configButton.Enable()
		configWindowOpen.Store(false)
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

	//* Start background goroutines — wrapped in recovery so a panic logs and exits cleanly
	withRecovery("hideOnClick", hideOnClick)
	withRecovery("updateDevices", updateDevices)
	withRecovery("monitorDeviceChanges", monitorDeviceChanges)

	//* Run the application event loop
	Win.ShowAndRun()
}

// . filterValidDevices filters out inaccessible devices while always including Remote Audio.
func filterValidDevices(devices []mmDeviceEnumerator.AudioDevice) []mmDeviceEnumerator.AudioDevice {
	valid := make([]mmDeviceEnumerator.AudioDevice, 0, len(devices))
	var remoteAudio *mmDeviceEnumerator.AudioDevice
	for _, device := range devices {
		if device.Name == "Remote Audio" {
			copy := device
			remoteAudio = &copy
			continue
		}
		if _, err := policyConfig.GetVolume(device.Id); err == nil {
			valid = append(valid, device)
		}
	}
	if remoteAudio != nil {
		valid = append(valid, *remoteAudio)
	}
	return valid
}

// . checkAndUpdateDevices checks for changes in the list of audio devices and updates the UI if changes are detected
func checkAndUpdateDevices() {
	//* Retrieve the current list of audio devices
	newAudioDevices, err := mmDeviceEnumerator.GetDevices()
	if err != nil || newAudioDevices == nil {
		fmt.Println("Error getting audio devices:", err)
		general.LogError("Error getting audio devices:", err)
		return
	}

	validDevices := filterValidDevices(newAudioDevices)

	//* Check for changes and whether config window is open — hold lock briefly
	mu.Lock()
	changed := !audioDevicesEqual(audioDevices, validDevices)
	open := configWindowOpen.Load()
	if changed && !open {
		audioDevices = validDevices
		currentDeviceID = ""
	}
	mu.Unlock()

	if changed && !open {
		loadSettings()
		fyne.Do(func() {
			renderButtons()
		})
		resize()
	}
}

// . updateDevices continuously checks for audio device changes at regular intervals
func updateDevices() {
	// Force an immediate fresh device list retrieval on startup
	freshDevices, err := mmDeviceEnumerator.GetDevices()
	if err == nil && freshDevices != nil {
		validDevices := filterValidDevices(freshDevices)

		mu.Lock()
		audioDevices = validDevices
		mu.Unlock()

		fyne.Do(func() {
			renderButtons()
		})
		resize()
	}

	// Set up the ticker for subsequent device updates
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		checkAndUpdateDevices()
	}
}

// . renderButtons dynamically creates and updates buttons for each audio device in the UI.
// Must be called on the Fyne goroutine (directly or inside fyne.Do).
func renderButtons() {
	//* Snapshot shared state under the lock so we don't hold it during UI work
	mu.Lock()
	devices := make([]mmDeviceEnumerator.AudioDevice, len(audioDevices))
	copy(devices, audioDevices)
	prevDeviceID := currentDeviceID
	mu.Unlock()

	newDeviceVbox := container.New(&fyneCustom.CustomVBoxLayout{FixedWidth: 150})

	// Track the new device ID and slider state we will commit at the end
	var newDeviceID string
	newSliderDisabled := false
	newSliderValue := float64(100)

	//* Create a button for each audio device
	for _, device := range devices {
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
				volume, err := policyConfig.GetVolume(device.Id)
				if err == nil {
					newDeviceID = device.Id
					newSliderDisabled = false
					newSliderValue = float64(volume * 100)
					muted, err := policyConfig.GetMute(device.Id)
					if err == nil && muted {
						newSliderValue = 0
					}
				} else {
					newDeviceID = device.Id
					newSliderDisabled = true
					newSliderValue = 100
				}
			} else {
				newDeviceVbox.Add(widget.NewButtonWithIcon(deviceName, theme.VolumeUpIcon(), onTapped))
				volume, err := policyConfig.GetVolume(device.Id)
				if err != nil {
					fmt.Printf("Error getting volume for device %s: %v\n", device.Id, err)
				} else {
					newDeviceID = device.Id
					newSliderDisabled = false
					newSliderValue = float64(volume * 100)
					muted, err := policyConfig.GetMute(device.Id)
					if err != nil {
						fmt.Printf("Error getting mute state for device %s: %v\n", device.Id, err)
					} else if muted {
						newSliderValue = 0
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
				// If Remote Audio was the previously selected device, keep slider disabled
				if prevDeviceID == device.Id {
					newSliderDisabled = true
					newSliderValue = 100
				}
			} else {
				newDeviceVbox.Add(widget.NewButton(deviceName, onTapped))
			}
		}
	}

	//* Commit the new device ID under the lock
	mu.Lock()
	currentDeviceID = newDeviceID
	mu.Unlock()

	//* Apply slider state
	volumeSlider.Disabled = newSliderDisabled
	if newSliderDisabled {
		volumeSlider.Disable()
	} else {
		volumeSlider.Enable()
	}
	volumeSlider.SetValue(newSliderValue)

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
	newSettings := AppSettings{
		HideAfterSelection: false,
		DeviceNames:        make(map[string]DeviceConfig),
	}

	//* Attempt to read the settings file from disk
	fileData, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			settings = newSettings
			saveSettings()
		}
		return
	}

	//* Parse settings from JSON file data
	json.Unmarshal(fileData, &newSettings)

	//* Ensure DeviceNames map is initialized even if missing from file
	if newSettings.DeviceNames == nil {
		newSettings.DeviceNames = make(map[string]DeviceConfig)
	}

	//* Build a map linking original device names to their IDs for device ID consistency
	originalNameToConfig := make(map[string]string)
	for id, config := range newSettings.DeviceNames {
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
		config, exists := newSettings.DeviceNames[device.Id]
		if !exists {
			if oldID, found := originalNameToConfig[device.Name]; found {
				config = newSettings.DeviceNames[oldID]
				updatedDeviceNames[device.Id] = config
				delete(newSettings.DeviceNames, oldID)
				fmt.Printf("Updated device ID for %s from %s to %s\n", device.Name, oldID, device.Id)
			} else {
				config = DeviceConfig{
					Name:         device.Name,
					IsShown:      true,
					OriginalName: device.Name,
				}
				updatedDeviceNames[device.Id] = config
			}
		} else {
			updatedDeviceNames[device.Id] = config
		}
	}

	newSettings.DeviceNames = updatedDeviceNames

	//* Commit the fully built settings atomically
	settings = newSettings
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
			if !file.Exists(startupPath) {
				general.CreateShortcut(file.Cwd()+"/soundshift.exe", startupPath)
			}
		} else {
			if file.Exists(startupPath) {
				os.Remove(startupPath)
			}
		}

		//* Save settings to file and close the configuration window
		saveSettings()
		configWin.Hide()
		configWindowOpen.Store(false)
		configButton.Enable()

		//* Refresh the main UI to reflect updated settings
		renderButtons()

		//* Resize after a short delay to let the Fyne layout catch up — done off the UI goroutine
		go func() {
			time.Sleep(100 * time.Millisecond)
			resize()
		}()
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

// . hideOnClick hides the application window if a mouse click occurs outside of it
func hideOnClick() {
	mouseChan := make(chan types.MouseEvent, 1024)
	mouse.Install(nil, mouseChan)
	defer mouse.Uninstall()

	//* Monitor mouse events for click actions
	for k := range mouseChan {
		if k.Message == 513 { // WM_LBUTTONDOWN
			//* Check if the click is outside the application window and taskbar
			if !isMouseInWindow() && !isMouseInTaskbar() && !configWindowOpen.Load() {
				winapi.HideWindow(hwnd)
			}
		}
	}
}

// . isMouseInWindow checks if the mouse cursor is currently within the application window boundaries
func isMouseInWindow() bool {
	xMouse, yMouse := robotgo.Location()
	xPos, yPos, _ := winapi.GetWindowPosition(hwnd)
	xSize, ySize, _ := winapi.GetWindowSize(hwnd)

	return int(xMouse) >= int(xPos) && int(xMouse) <= int(xPos+xSize) &&
		int(yMouse) >= int(yPos) && int(yMouse) <= int(yPos+ySize)
}

// . isMouseInTaskbar checks if the mouse cursor is currently within the taskbar area
func isMouseInTaskbar() bool {
	_, yMouse := robotgo.Location()
	currentScreenHeight := int(win.GetSystemMetrics(win.SM_CYSCREEN))
	return currentScreenHeight-yMouse <= winapi.GetTaskbarHeight()
}

// . audioDevicesEqual performs a fast comparison of AudioDevice slices without reflection
func audioDevicesEqual(a, b []mmDeviceEnumerator.AudioDevice) bool {
	if len(a) != len(b) {
		return false
	}
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
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

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

		// Read shared state under lock
		mu.Lock()
		devID := currentDeviceID
		devices := make([]mmDeviceEnumerator.AudioDevice, len(audioDevices))
		copy(devices, audioDevices)
		mu.Unlock()

		if devID == "" {
			continue
		}

		// Reset cache if device changed
		if devID != lastDeviceID {
			lastVolume = -1
			lastMuted = false
			lastDeviceID = devID
			isRemoteAudioInaccessible = false
		}

		// Validate that the current device still exists in the device list
		deviceExists := false
		var currentDevice mmDeviceEnumerator.AudioDevice
		for _, device := range devices {
			if device.Id == devID {
				deviceExists = true
				currentDevice = device
				break
			}
		}

		// If device no longer exists, reset currentDeviceID and skip monitoring
		if !deviceExists {
			fmt.Printf("Device %s no longer exists, resetting current device\n", devID)
			mu.Lock()
			if currentDeviceID == devID {
				currentDeviceID = ""
			}
			mu.Unlock()
			lastDeviceID = ""
			continue
		}

		// Skip volume/mute polling for inaccessible Remote Audio devices
		if currentDevice.Name == "Remote Audio" && !isRemoteAudioInaccessible {
			_, err := policyConfig.GetVolume(devID)
			if err != nil {
				isRemoteAudioInaccessible = true
				fyne.Do(func() {
					volumeSlider.Disabled = true
					volumeSlider.Disable()
					volumeSlider.SetValue(100)
				})
				continue
			}
		}

		if currentDevice.Name == "Remote Audio" && isRemoteAudioInaccessible {
			continue
		}

		// Retrieve current volume and mute state
		volume, err := policyConfig.GetVolume(devID)
		if err != nil {
			if currentDevice.Name == "Remote Audio" {
				isRemoteAudioInaccessible = true
				fyne.Do(func() {
					volumeSlider.Disabled = true
					volumeSlider.Disable()
					volumeSlider.SetValue(100)
				})
				continue
			}
			fmt.Printf("Error getting volume for device %s: %v\n", devID, err)
			mu.Lock()
			if currentDeviceID == devID {
				currentDeviceID = ""
			}
			mu.Unlock()
			lastDeviceID = ""
			continue
		}

		muted, err := policyConfig.GetMute(devID)
		if err != nil {
			if currentDevice.Name == "Remote Audio" {
				isRemoteAudioInaccessible = true
				fyne.Do(func() {
					volumeSlider.Disabled = true
					volumeSlider.Disable()
					volumeSlider.SetValue(100)
				})
				continue
			}
			fmt.Printf("Error getting mute state for device %s: %v\n", devID, err)
			mu.Lock()
			if currentDeviceID == devID {
				currentDeviceID = ""
			}
			mu.Unlock()
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

		// Optimistically update IsDefault flags for immediate visual feedback
		mu.Lock()
		for i := range audioDevices {
			audioDevices[i].IsDefault = (audioDevices[i].Id == deviceID)
		}
		mu.Unlock()
		renderButtons()

		// After OS has had time to settle, confirm with a fresh device list
		go func() {
			time.Sleep(200 * time.Millisecond)
			newDevices, err := mmDeviceEnumerator.GetDevices()
			if err == nil && newDevices != nil {
				mu.Lock()
				audioDevices = newDevices
				mu.Unlock()
				fyne.Do(func() {
					renderButtons()
				})
			}
			if settings.HideAfterSelection {
				winapi.HideWindow(hwnd)
			}
		}()
	}
}
