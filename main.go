package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"runtime"
	"soundshift/file"
	"soundshift/fyneCustom"
	"soundshift/fyneTheme"
	"soundshift/general"
	"soundshift/interfaces/mmDeviceEnumerator"
	"soundshift/interfaces/policyConfig"
	"soundshift/winapi"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

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
	HideAfterSelection     bool
	RememberScrollPosition bool
	HiddenApps             map[string]bool // key = lowercase exe name (e.g. "firefox")
	DeviceNames            map[string]DeviceConfig
}

// . Global variables for application state and configuration
var lastHideTime atomic.Int64    // unix-nano timestamp of last hideMainWindow call
var configWindowOpen atomic.Bool // replaces plain bool; safe for concurrent read from goroutines
var settings AppSettings
var currentDeviceID string
var audioDevices []mmDeviceEnumerator.AudioDevice
var windowPadding = 20
var screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
var screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
var taskbarHeight = winapi.GetTaskbarHeight()
var hwnd windows.HWND
var configHwnd windows.HWND
var deviceVboxPlaceholder = container.New(&fyneCustom.CustomVBoxLayout{FixedWidth: 150})
var placementAreaMu sync.RWMutex
var placementArea winapi.RECT
var placementAreaSet bool

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

// . waitForHWNDByTitle waits for a valid window handle with timeout and returns it
func waitForHWNDByTitle(pid uint32, title string, timeout time.Duration) (windows.HWND, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h, err := winapi.GetHwnd(pid, title)
		if err == nil && h != 0 {
			general.LogError(fmt.Sprintf("HWND_ACQUIRED (%s hwnd=%d)", title, uintptr(h)), nil)
			return h, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0, fmt.Errorf("timeout acquiring hwnd for %q", title)
}

func hideMainWindow() {
	if hwnd != 0 {
		// Only record the hide timestamp when actually hiding a visible window,
		// so the debounce in toggleWindow doesn't suppress a legitimate show.
		if winapi.IsWindowVisible(hwnd) {
			lastHideTime.Store(time.Now().UnixNano())
		}
		winapi.HideWindow(hwnd)
	}
	fyne.Do(func() {
		Win.Hide()
	})
}

func showMainWindow() {
	// Scroll to top unless the user opted to remember scroll position.
	if !settings.RememberScrollPosition {
		fyne.Do(func() {
			mainView.ScrollToTop()
		})
	}
	if hwnd != 0 {
		winapi.ShowWindow(hwnd)
		winapi.SetTopmost(hwnd)
		// Apply rounded corners after showing the window
		winapi.SetRoundedCorners(hwnd, 20)
	}
	done := make(chan struct{})
	fyne.Do(func() {
		Win.Show()
		close(done)
	})
	<-done
}

// . toggleWindow toggles the main window visibility
func toggleWindow() {
	if hwnd == 0 {
		general.LogError("toggle with hwnd==0", fmt.Errorf("no hwnd"))
		return
	}
	if winapi.IsWindowVisible(hwnd) {
		general.LogError("TOGGLE_HIDE", nil)
		hideMainWindow()
	} else {
		// If the window was hidden very recently (e.g. by the global mouse hook on
		// the same physical click that triggered this tray-icon callback), don't
		// immediately re-show it. This lets a single tray-icon click cleanly hide
		// the window instead of hide-then-reshow.
		if time.Since(time.Unix(0, lastHideTime.Load())) < 300*time.Millisecond {
			general.LogError("TOGGLE_SHOW_SUPPRESSED", nil)
			return
		}
		general.LogError("TOGGLE_SHOW", nil)
		setPlacementAreaForCursor()

		// Position the window before showing it to minimize flicker
		resize()
		showMainWindow()

		// Run several correction passes to handle late size adjustments on first show.
		go func() {
			for _, delay := range []time.Duration{0, 30 * time.Millisecond, 80 * time.Millisecond, 160 * time.Millisecond, 280 * time.Millisecond} {
				time.Sleep(delay)
				if !winapi.IsWindowVisible(hwnd) {
					return
				}
				resize()
			}
		}()
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

// * Top section: device selector + master volume
var topSection = container.NewCenter(
	container.NewVBox(
		deviceVboxPlaceholder,
		&canvas.Line{StrokeColor: color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xCC}, StrokeWidth: 1},
		configButton,
		container.NewPadded(volumeSlider),
	),
)

// * Mixer section: per-app volume sliders
var mixerVbox = container.NewVBox()
var mixerSection = container.NewVBox() // populated when sessions exist

// * Inner content: everything that can be scrolled
var mainViewInner = container.NewPadded(
	container.NewVBox(
		topSection,
		mixerSection,
	),
)

// * Main view: scrollable wrapper
var mainView = container.NewVScroll(mainViewInner)

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
			if configHwnd == 0 {
				go func() {
					h, err := waitForHWNDByTitle(windows.GetCurrentProcessId(), "Configure", 3*time.Second)
					if err == nil {
						configHwnd = h
						winapi.EnableAcrylic(h)
					}
				}()
			}
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

func setPlacementAreaForCursor() {
	x, y := robotgo.Location()
	workArea, err := winapi.GetWorkAreaFromPoint(int32(x), int32(y))
	if err != nil {
		return
	}

	placementAreaMu.Lock()
	placementArea = workArea
	placementAreaSet = true
	placementAreaMu.Unlock()
}

func getPlacementArea() (winapi.RECT, bool) {
	placementAreaMu.RLock()
	defer placementAreaMu.RUnlock()
	return placementArea, placementAreaSet
}

func positionWindow(physW, physH int32) {
	if hwnd == 0 {
		return
	}
	workArea, ok := getPlacementArea()
	if !ok {
		var err error
		workArea, err = winapi.GetWorkArea()
		if err != nil {
			refreshScreenMetrics()
			workArea = winapi.RECT{Left: 0, Top: 0, Right: int32(screenWidth), Bottom: int32(screenHeight - taskbarHeight)}
		}
	}

	x := int(workArea.Right) - int(physW) - windowPadding
	y := int(workArea.Bottom) - int(physH) - windowPadding

	// Clamp bounds include the padding so the window never sits flush against an edge.
	minX := int(workArea.Left) + windowPadding
	minY := int(workArea.Top) + windowPadding
	maxX := int(workArea.Right) - int(physW) - windowPadding
	maxY := int(workArea.Bottom) - int(physH) - windowPadding

	if maxX < minX {
		maxX = minX
	}
	if maxY < minY {
		maxY = minY
	}
	if x < minX {
		x = minX
	} else if x > maxX {
		x = maxX
	}
	if y < minY {
		y = minY
	} else if y > maxY {
		y = maxY
	}

	// Use optimized positioning to minimize flicker during frequent repositioning
	winapi.PositionWindowOptimized(
		hwnd,
		int32(x),
		int32(y),
		physW,
		physH,
	)
	winapi.SetRoundedCorners(hwnd, 20)
}

// maxWindowHeight is the maximum height (in dp) the window will grow to.
// Content beyond this is reachable by scrolling.
const maxWindowHeight float32 = 500

func resizeOnUI() {
	// Size the window to fit only the top section (device buttons + master
	// slider). The mixer section lives below and is reachable by scrolling.
	topSize := topSection.MinSize()

	// Add title-bar height + padding from mainViewInner's NewPadded wrapper.
	const titleBarH float32 = 34 // matches TitleBar.MinSize().Height
	const padExtra float32 = 16  // NewPadded adds theme.Padding()*2 ≈ 8*2
	totalW := topSize.Width + padExtra
	totalH := topSize.Height + titleBarH + padExtra

	if totalH > maxWindowHeight {
		totalH = maxWindowHeight
	}

	scale := float64(1)
	if Win.Canvas() != nil {
		scale = float64(Win.Canvas().Scale())
	}
	physW := int32(math.Ceil(float64(totalW) * scale))
	physH := int32(math.Ceil(float64(totalH) * scale))
	positionWindow(physW, physH)

	actualW, actualH, err := winapi.GetWindowSize(hwnd)
	if err == nil && actualW > 0 && actualH > 0 && (actualW != physW || actualH != physH) {
		positionWindow(actualW, actualH)
	}
}

func resize() {
	if hwnd == 0 {
		return
	}
	done := make(chan struct{})
	fyne.Do(func() {
		resizeOnUI()
		close(done)
	})
	<-done
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

	//* Build custom title bar with unified drag, alignment, and close hover effect
	titleBar := fyneCustom.NewTitleBar(
		fyne.NewStaticResource("icon", icon),
		"SoundShift",
		func() { hideMainWindow() },
		func() { winapi.StartWindowDrag(hwnd) },
	)

	//* Configure main window properties and layout
	Win.SetContent(container.NewBorder(titleBar, nil, nil, nil, mainView))
	Win.SetTitle(title)
	Win.SetIcon(fyne.NewStaticResource("icon", icon))
	Win.SetCloseIntercept(func() {
		//* Intercept window close to hide it instead of terminating the app
		hideMainWindow()
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
	h, err := waitForHWNDByTitle(pid, title, 5*time.Second)
	if err != nil {
		general.LogError("Failed to acquire HWND", err)
		return
	}
	hwnd = h

	//* Apply Windows API settings to the application window
	winapi.HideTitleBar(hwnd) // remove native chrome before resize so MinSize is correct
	resize()
	winapi.HideWindow(hwnd)
	done := make(chan struct{})
	fyne.Do(func() {
		Win.Hide()
		close(done)
	})
	<-done
	winapi.HideMinMaxButtons(hwnd)
	winapi.HideWindowFromTaskbar(hwnd)
	winapi.SetTopmost(hwnd)
	winapi.EnableAcrylic(hwnd)

	// Pre-settle placement while hidden so the very first user-visible show opens correctly.
	go func() {
		for _, delay := range []time.Duration{40 * time.Millisecond, 120 * time.Millisecond, 250 * time.Millisecond, 450 * time.Millisecond} {
			time.Sleep(delay)
			if hwnd == 0 {
				return
			}
			resize()
		}
	}()

	//* Start systray on a locked OS thread
	go func() {
		runtime.LockOSThread()
		systray.Run(initTray, func() {})
	}()

	//* Start background goroutines — wrapped in recovery so a panic logs and exits cleanly
	withRecovery("hideOnClick", hideOnClick)
	withRecovery("monitorFocusLoss", monitorFocusLoss)
	withRecovery("updateDevices", updateDevices)
	withRecovery("monitorDeviceChanges", monitorDeviceChanges)
	withRecovery("monitorMixer", monitorMixer)

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
			if winapi.IsWindowVisible(hwnd) {
				resizeOnUI()
			}
		})
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
			if winapi.IsWindowVisible(hwnd) {
				resizeOnUI()
			}
		})
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
		HideAfterSelection:     false,
		RememberScrollPosition: false,
		HiddenApps:             make(map[string]bool),
		DeviceNames:            make(map[string]DeviceConfig),
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
	if newSettings.HiddenApps == nil {
		newSettings.HiddenApps = make(map[string]bool)
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
		resetButton := fyneCustom.NewColorButton("", color.RGBA{68, 72, 81, 192}, theme.MediaReplayIcon(), func() {
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

	//* Checkbox for remembering scroll position on show
	rememberScrollCheckbox := &widget.Check{
		Text:    "Remember scroll position on show",
		Checked: settings.RememberScrollPosition,
	}

	//* Checkbox for setting the application to start with Windows
	startWithWindowsCheckbox := &widget.Check{
		Text:    "Start with Windows",
		Checked: file.Exists(file.RoamingDir() + "/Microsoft/Windows/Start Menu/Programs/Startup/soundshift.lnk"),
	}

	//* Hidden apps section — gather known app names from current sessions + saved hidden apps
	knownApps := make(map[string]bool) // lowercase name → true
	for name := range settings.HiddenApps {
		knownApps[strings.ToLower(name)] = true
	}
	if currentSessions, err := policyConfig.GetAudioSessions(); err == nil {
		for _, s := range currentSessions {
			knownApps[strings.ToLower(s.Name)] = true
		}
	}
	// Build sorted list and checkboxes
	type hiddenAppEntry struct {
		name     string
		checkbox *widget.Check
	}
	var hiddenAppEntries []hiddenAppEntry
	for name := range knownApps {
		displayName := name
		// Capitalize first letter for display
		if len(displayName) > 0 {
			displayName = strings.ToUpper(displayName[:1]) + displayName[1:]
		}
		isHidden := settings.HiddenApps[name]
		cb := &widget.Check{
			Text:    displayName,
			Checked: isHidden,
		}
		hiddenAppEntries = append(hiddenAppEntries, hiddenAppEntry{name: name, checkbox: cb})
	}
	// Sort alphabetically
	for i := 0; i < len(hiddenAppEntries); i++ {
		for j := i + 1; j < len(hiddenAppEntries); j++ {
			if hiddenAppEntries[i].name > hiddenAppEntries[j].name {
				hiddenAppEntries[i], hiddenAppEntries[j] = hiddenAppEntries[j], hiddenAppEntries[i]
			}
		}
	}
	hiddenAppsBox := container.NewVBox()
	if len(hiddenAppEntries) > 0 {
		hiddenAppsLabel := canvas.NewText("Hide from mixer:", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xb0})
		hiddenAppsLabel.TextSize = 12
		hiddenAppsBox.Add(hiddenAppsLabel)
		for _, entry := range hiddenAppEntries {
			hiddenAppsBox.Add(entry.checkbox)
		}
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
		settings.RememberScrollPosition = rememberScrollCheckbox.Checked

		//* Update hidden apps from checkboxes
		newHiddenApps := make(map[string]bool)
		for _, entry := range hiddenAppEntries {
			if entry.checkbox.Checked {
				newHiddenApps[entry.name] = true
			}
		}
		settings.HiddenApps = newHiddenApps

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

		//* Force-rebuild the mixer so hidden-app changes take effect immediately
		mixerSessionKeys = nil
		refreshMixer()

		//* Resize after a short delay to let the Fyne layout catch up — done off the UI goroutine
		go func() {
			time.Sleep(100 * time.Millisecond)
			fyne.Do(func() {
				if winapi.IsWindowVisible(hwnd) {
					resizeOnUI()
				}
			})
		}()
	})

	//* Layout for save button and checkboxes
	saveButtonContainer := container.New(layout.NewCenterLayout(), saveButton)
	checkboxAndButtonVBox := container.NewVBox(hideAfterSelectionCheckbox, rememberScrollCheckbox, startWithWindowsCheckbox)
	if len(hiddenAppEntries) > 0 {
		checkboxAndButtonVBox.Add(widget.NewSeparator())
		checkboxAndButtonVBox.Add(hiddenAppsBox)
	}
	checkboxAndButtonVBox.Add(saveButtonContainer)
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
				hideMainWindow()
			}
		}
	}
}

// . monitorFocusLoss hides the window when it loses foreground status.
// This catches cases the mouse hook misses, e.g. clicks inside fullscreen game windows
// that use DirectInput/raw input and bypass the low-level mouse hook.
func monitorFocusLoss() {
	var wasForeground bool
	for {
		time.Sleep(100 * time.Millisecond)
		if hwnd == 0 {
			wasForeground = false
			continue
		}
		fg := winapi.GetForegroundWindow()
		isForeground := fg == hwnd || fg == configHwnd
		if wasForeground && !isForeground && winapi.IsWindowVisible(hwnd) && !configWindowOpen.Load() {
			hideMainWindow()
		}
		wasForeground = isForeground
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

// . isMouseInTaskbar checks if the mouse cursor is currently within the taskbar area on any monitor
func isMouseInTaskbar() bool {
	xMouse, yMouse := robotgo.Location()
	workArea, err := winapi.GetWorkAreaFromPoint(int32(xMouse), int32(yMouse))
	if err != nil {
		// Fallback: primary-monitor check
		currentScreenHeight := int(win.GetSystemMetrics(win.SM_CYSCREEN))
		return currentScreenHeight-yMouse <= winapi.GetTaskbarHeight()
	}
	// Cursor is in the taskbar/docked-bar zone if it lies outside the work area
	// of the nearest monitor (the work area excludes the taskbar).
	mx, my := int32(xMouse), int32(yMouse)
	return mx < workArea.Left || mx >= workArea.Right || my < workArea.Top || my >= workArea.Bottom
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
				// Skip external updates while the user is dragging the slider so the
				// ticker cannot snap the thumb back mid-drag (which caused the visible
				// "back and forth" oscillation).
				if volumeSlider.IsDragging() {
					return
				}
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
				hideMainWindow()
			}
		}()
	}
}

// ---------------------------------------------------------------------------
// Icon extraction from executables (Windows shell32 / gdi32)
// ---------------------------------------------------------------------------

var (
	shell32DLL         = windows.NewLazySystemDLL("shell32.dll")
	gdi32DLL           = windows.NewLazySystemDLL("gdi32.dll")
	procExtractIconExW = shell32DLL.NewProc("ExtractIconExW")
	procDestroyIcon    = windows.NewLazySystemDLL("user32.dll").NewProc("DestroyIcon")
	procGetIconInfo    = windows.NewLazySystemDLL("user32.dll").NewProc("GetIconInfo")
	procGetDIBits      = gdi32DLL.NewProc("GetDIBits")
	procCreateCompatDC = gdi32DLL.NewProc("CreateCompatibleDC")
	procDeleteDC       = gdi32DLL.NewProc("DeleteDC")
	procDeleteObject   = gdi32DLL.NewProc("DeleteObject")
	procGetObjectW     = gdi32DLL.NewProc("GetObjectW")
)

type bitmapStruct struct {
	BmType       int32
	BmWidth      int32
	BmHeight     int32
	BmWidthBytes int32
	BmPlanes     uint16
	BmBitsPixel  uint16
	BmBits       uintptr
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

// iconCache caches fyne.Resource icons by exe path to avoid repeated extraction.
var iconCache sync.Map // map[string]fyne.Resource

// extractAppIcon extracts the first icon from the executable at exePath and
// returns it as a fyne.Resource (PNG). Returns nil on failure.
func extractAppIcon(exePath string) fyne.Resource {
	if exePath == "" {
		return nil
	}
	// Check cache first.
	if cached, ok := iconCache.Load(exePath); ok {
		if cached == nil {
			return nil
		}
		return cached.(fyne.Resource)
	}

	res := doExtractIcon(exePath)
	iconCache.Store(exePath, res)
	return res
}

func doExtractIcon(exePath string) fyne.Resource {
	pathPtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return nil
	}

	var hIconLarge uintptr
	ret, _, _ := procExtractIconExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&hIconLarge)),
		0, // no small icon
		1,
	)
	if ret == 0 || hIconLarge == 0 {
		return nil
	}
	defer procDestroyIcon.Call(hIconLarge)

	// Get icon info to access the color bitmap.
	var ii iconInfo
	r1, _, _ := procGetIconInfo.Call(hIconLarge, uintptr(unsafe.Pointer(&ii)))
	if r1 == 0 {
		return nil
	}
	if ii.HbmMask != 0 {
		defer procDeleteObject.Call(ii.HbmMask)
	}
	if ii.HbmColor == 0 {
		return nil
	}
	defer procDeleteObject.Call(ii.HbmColor)

	// Get bitmap dimensions.
	var bm bitmapStruct
	procGetObjectW.Call(ii.HbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	if bm.BmWidth == 0 || bm.BmHeight == 0 {
		return nil
	}

	w, h := int(bm.BmWidth), int(bm.BmHeight)

	// Prepare BITMAPINFOHEADER for GetDIBits (top-down by using negative height).
	bih := bitmapInfoHeader{
		BiSize:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		BiWidth:    int32(w),
		BiHeight:   -int32(h), // negative = top-down
		BiPlanes:   1,
		BiBitCount: 32,
	}

	pixels := make([]byte, w*h*4)

	hdc, _, _ := procCreateCompatDC.Call(0)
	if hdc == 0 {
		return nil
	}
	defer procDeleteDC.Call(hdc)

	procGetDIBits.Call(
		hdc,
		ii.HbmColor,
		0,
		uintptr(h),
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&bih)),
		0, // DIB_RGB_COLORS
	)

	// Convert BGRA → RGBA.
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := (y*w + x) * 4
			img.SetRGBA(x, y, color.RGBA{
				R: pixels[off+2],
				G: pixels[off+1],
				B: pixels[off+0],
				A: pixels[off+3],
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}

	return fyne.NewStaticResource(exePath, buf.Bytes())
}

// . refreshMixer rebuilds the per-app volume mixer UI from current audio sessions.
// Must be called on the Fyne goroutine.
func refreshMixer() {
	sessions, err := policyConfig.GetAudioSessions()
	if err != nil {
		mixerSection.Objects = nil
		mixerSection.Refresh()
		return
	}

	if len(sessions) == 0 {
		mixerSection.Objects = nil
		mixerSection.Refresh()
		return
	}

	// Filter out apps the user has hidden via settings.
	if len(settings.HiddenApps) > 0 {
		filtered := sessions[:0]
		for _, s := range sessions {
			key := strings.ToLower(s.Name)
			if !settings.HiddenApps[key] {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	if len(sessions) == 0 {
		mixerSection.Objects = nil
		mixerSection.Refresh()
		return
	}

	// Build a key→session map for the new snapshot.
	type sessionKey = struct {
		pid   uint32
		name  string
		isSys bool
	}
	newKeys := make([]sessionKey, len(sessions))
	for i, s := range sessions {
		newKeys[i] = sessionKey{pid: s.PID, name: s.Name, isSys: s.IsSystem}
	}

	// Check if the set of sessions changed compared to last time.
	sessionsChanged := len(newKeys) != len(mixerSessionKeys)
	if !sessionsChanged {
		for i := range newKeys {
			if newKeys[i] != mixerSessionKeys[i] {
				sessionsChanged = true
				break
			}
		}
	}

	if sessionsChanged {
		// Rebuild the entire mixer UI.
		mixerSessionKeys = newKeys
		mixerSliders = make([]*fyneCustom.ScrollableSlider, len(sessions))
		mixerMuteButtons = make([]*fyneCustom.IconButton, len(sessions))
		mixerMuted = make([]bool, len(sessions))

		newMixer := container.NewVBox()

		for i, sess := range sessions {
			sessIdx := sess.SessionIdx
			sessI := i
			displayName := general.EllipticalTruncate(sess.Name, 24)

			// --- App icon ---
			var appIcon *canvas.Image
			if iconRes := extractAppIcon(sess.ExePath); iconRes != nil {
				appIcon = canvas.NewImageFromResource(iconRes)
			} else {
				// Fallback: use a generic speaker icon for system sounds or unknown apps
				appIcon = canvas.NewImageFromResource(theme.VolumeUpIcon())
			}
			appIcon.FillMode = canvas.ImageFillContain
			appIcon.SetMinSize(fyne.NewSize(16, 16))

			// --- Name label ---
			label := canvas.NewText(displayName, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xd0})
			label.TextSize = 11

			// --- Mute button ---
			muted := sess.Muted
			mixerMuted[i] = muted
			muteBtn := fyneCustom.NewIconButton(theme.VolumeMuteIcon(), nil)
			muteBtn.SetActive(muted)
			muteBtn.OnTapped = func() {
				newMuted := !mixerMuted[sessI]
				// Optimistic UI update — instant feedback before COM call completes
				mixerMuted[sessI] = newMuted
				muteBtn.SetActive(newMuted)
				go func() {
					if err := policyConfig.SetSessionMute(sessIdx, newMuted); err != nil {
						general.LogError("Error setting session mute", err)
					}
				}()
			}
			mixerMuteButtons[i] = muteBtn

			// --- Slider — always show the real underlying volume, mute is handled separately ---
			slider := fyneCustom.NewScrollableSlider(0, 100)
			slider.SetValue(float64(sess.Volume * 100))

			slider.OnChanged = func(f float64) {
				level := float32(f / 100.0)
				go func() {
					if err := policyConfig.SetSessionVolume(sessIdx, level); err != nil {
						general.LogError("Error setting session volume", err)
					}
				}()
			}

			mixerSliders[i] = slider

			// Layout: top row = [icon] name ... [muteBtn]
			//         bottom  = slider (full width)
			iconContainer := container.NewCenter(appIcon)
			topRow := container.NewBorder(nil, nil,
				container.NewHBox(iconContainer, label),
				muteBtn,
			)
			entry := container.NewVBox(topRow, slider)
			newMixer.Add(entry)
		}

		mixerSection.Objects = []fyne.CanvasObject{newMixer}
		mixerSection.Refresh()

		// Reposition the window after Fyne has had time to lay out the new
		// mixer content. An immediate call would use stale dimensions; the
		// delayed passes let Fyne settle first.
		go func() {
			for _, delay := range []time.Duration{50 * time.Millisecond, 150 * time.Millisecond, 300 * time.Millisecond} {
				time.Sleep(delay)
				if !winapi.IsWindowVisible(hwnd) {
					return
				}
				fyne.Do(func() {
					resizeOnUI()
				})
			}
		}()
	} else {
		// Sessions haven't changed — just update slider values and mute state in-place.
		for i, sess := range sessions {
			if i >= len(mixerSliders) || mixerSliders[i] == nil {
				continue
			}
			// Sync mute button active state if changed externally.
			if i < len(mixerMuted) && i < len(mixerMuteButtons) && mixerMuteButtons[i] != nil {
				if sess.Muted != mixerMuted[i] {
					mixerMuted[i] = sess.Muted
					mixerMuteButtons[i].SetActive(sess.Muted)
				}
			}
			// Don't override the slider while the user is dragging it.
			if mixerSliders[i].IsDragging() {
				continue
			}
			// Always track the real underlying volume; mute state is shown separately.
			target := float64(sess.Volume * 100)
			if mixerSliders[i].Value != target {
				mixerSliders[i].SetValue(target)
			}
		}
	}
}

// mixer state tracking for in-place updates
type mixerKey struct {
	pid   uint32
	name  string
	isSys bool
}

var mixerSessionKeys []struct {
	pid   uint32
	name  string
	isSys bool
}
var mixerSliders []*fyneCustom.ScrollableSlider
var mixerMuteButtons []*fyneCustom.IconButton
var mixerMuted []bool

// . monitorMixer periodically refreshes the per-app mixer UI and updates slider
// values to reflect external volume changes.
func monitorMixer() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !winapi.IsWindowVisible(hwnd) {
			continue
		}

		fyne.Do(func() {
			refreshMixer()
		})
	}
}
