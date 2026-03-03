package policyConfig

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/moutend/go-wca/pkg/wca"
	"golang.org/x/sys/windows"
)

// AudioSession represents a single per-application audio session on the default
// render device, exposing the information needed for a mixer UI.
type AudioSession struct {
	Name       string  // Friendly display name (process executable name or "System Sounds")
	PID        uint32  // Process ID (0 for system sounds)
	Volume     float32 // Master volume scalar [0.0 – 1.0]
	Muted      bool    // Whether the session is muted
	IsSystem   bool    // True when the session represents system sounds
	SessionIdx int     // Index inside the enumerator (used to re-acquire the session for Set calls)
	ExePath    string  // Full path to the executable (empty for system sounds)
}

// GetAudioSessions enumerates all active audio sessions on the default render
// device and returns a slice of AudioSession structs.
func GetAudioSessions() ([]AudioSession, error) {
	var deviceEnumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator, &deviceEnumerator,
	); err != nil {
		return nil, fmt.Errorf("create device enumerator: %w", err)
	}
	defer deviceEnumerator.Release()

	var defaultDevice *wca.IMMDevice
	if err := deviceEnumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &defaultDevice); err != nil {
		return nil, fmt.Errorf("get default endpoint: %w", err)
	}
	defer defaultDevice.Release()

	var sessionManager *wca.IAudioSessionManager2
	if err := defaultDevice.Activate(
		wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, nil, &sessionManager,
	); err != nil {
		return nil, fmt.Errorf("activate session manager: %w", err)
	}
	defer sessionManager.Release()

	var enumerator *wca.IAudioSessionEnumerator
	if err := sessionManager.GetSessionEnumerator(&enumerator); err != nil {
		return nil, fmt.Errorf("get session enumerator: %w", err)
	}
	defer enumerator.Release()

	var count int
	if err := enumerator.GetCount(&count); err != nil {
		return nil, fmt.Errorf("get session count: %w", err)
	}

	sessions := make([]AudioSession, 0, count)
	for i := 0; i < count; i++ {
		var ctrl *wca.IAudioSessionControl
		if err := enumerator.GetSession(i, &ctrl); err != nil {
			continue
		}

		// Query IAudioSessionControl2 for PID / system-sounds flag.
		var ctrl2 *wca.IAudioSessionControl2
		if err := ctrl.PutQueryInterface(wca.IID_IAudioSessionControl2, &ctrl2); err != nil {
			ctrl.Release()
			continue
		}

		// Query ISimpleAudioVolume for volume / mute.
		var vol *wca.ISimpleAudioVolume
		if err := ctrl.PutQueryInterface(wca.IID_ISimpleAudioVolume, &vol); err != nil {
			ctrl2.Release()
			ctrl.Release()
			continue
		}

		// Check session state – only include active sessions (state == 1).
		var state uint32
		if err := ctrl.GetState(&state); err != nil || state != 1 {
			vol.Release()
			ctrl2.Release()
			ctrl.Release()
			continue
		}

		var pid uint32
		_ = ctrl2.GetProcessId(&pid)

		isSystem := ctrl2.IsSystemSoundsSession() == nil

		// Resolve the executable path; skip system host processes that aren't
		// meaningful to show in a per-app mixer (svchost, RuntimeBroker, etc.).
		exePath := exePathForPID(pid)
		bareName := bareExeName(exePath)
		if shouldFilterSession(bareName) && !isSystem {
			vol.Release()
			ctrl2.Release()
			ctrl.Release()
			continue
		}

		var level float32
		_ = vol.GetMasterVolume(&level)

		var muted bool
		_ = vol.GetMute(&muted)

		name := friendlyName(exePath, bareName)
		if isSystem {
			name = "System Sounds"
		}

		sessions = append(sessions, AudioSession{
			Name:       name,
			PID:        pid,
			Volume:     level,
			Muted:      muted,
			IsSystem:   isSystem,
			SessionIdx: i,
			ExePath:    exePath,
		})

		vol.Release()
		ctrl2.Release()
		ctrl.Release()
	}

	return sessions, nil
}

// SetSessionVolume sets the volume for the audio session at the given enumerator index.
func SetSessionVolume(sessionIdx int, level float32) error {
	vol, release, err := acquireSessionVolume(sessionIdx)
	if err != nil {
		return err
	}
	defer release()
	return vol.SetMasterVolume(level, nil)
}

// SetSessionMute sets the mute state for the audio session at the given enumerator index.
func SetSessionMute(sessionIdx int, muted bool) error {
	vol, release, err := acquireSessionVolume(sessionIdx)
	if err != nil {
		return err
	}
	defer release()
	return vol.SetMute(muted, nil)
}

// acquireSessionVolume opens the COM chain down to ISimpleAudioVolume for the
// given session index. The caller must invoke the returned release func.
func acquireSessionVolume(sessionIdx int) (*wca.ISimpleAudioVolume, func(), error) {
	var deviceEnumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator, &deviceEnumerator,
	); err != nil {
		return nil, nil, err
	}

	var defaultDevice *wca.IMMDevice
	if err := deviceEnumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &defaultDevice); err != nil {
		deviceEnumerator.Release()
		return nil, nil, err
	}

	var sessionManager *wca.IAudioSessionManager2
	if err := defaultDevice.Activate(
		wca.IID_IAudioSessionManager2, wca.CLSCTX_ALL, nil, &sessionManager,
	); err != nil {
		defaultDevice.Release()
		deviceEnumerator.Release()
		return nil, nil, err
	}

	var enumerator *wca.IAudioSessionEnumerator
	if err := sessionManager.GetSessionEnumerator(&enumerator); err != nil {
		sessionManager.Release()
		defaultDevice.Release()
		deviceEnumerator.Release()
		return nil, nil, err
	}

	var ctrl *wca.IAudioSessionControl
	if err := enumerator.GetSession(sessionIdx, &ctrl); err != nil {
		enumerator.Release()
		sessionManager.Release()
		defaultDevice.Release()
		deviceEnumerator.Release()
		return nil, nil, err
	}

	var vol *wca.ISimpleAudioVolume
	if err := ctrl.PutQueryInterface(wca.IID_ISimpleAudioVolume, &vol); err != nil {
		ctrl.Release()
		enumerator.Release()
		sessionManager.Release()
		defaultDevice.Release()
		deviceEnumerator.Release()
		return nil, nil, err
	}

	release := func() {
		vol.Release()
		ctrl.Release()
		enumerator.Release()
		sessionManager.Release()
		defaultDevice.Release()
		deviceEnumerator.Release()
	}

	return vol, release, nil
}

// processName returns a friendly display name for a PID.
func processName(pid uint32) string {
	p := exePathForPID(pid)
	return friendlyName(p, bareExeName(p))
}

// filteredExeNames is the set of system host executables that should be hidden
// from the per-app volume mixer.
var filteredExeNames = map[string]bool{
	"svchost":                 true,
	"runtimebroker":           true,
	"backgroundtaskhost":      true,
	"sihost":                  true,
	"systemsettings":          true,
	"systemsettingsbroker":    true,
	"searchhost":              true,
	"taskhostw":               true,
	"dllhost":                 true,
	"applicationframehost":    true,
	"shellexperiencehost":     true,
	"startmenuexperiencehost": true,
	"ctfmon":                  true,
	"conhost":                 true,
}

// shouldFilterSession returns true if the given bare exe name (no ext, lower)
// should be hidden from the mixer.
func shouldFilterSession(bareName string) bool {
	if bareName == "" {
		return true
	}
	return filteredExeNames[strings.ToLower(bareName)]
}

// exePathForPID returns the full executable path for a PID. Returns "" on failure.
func exePathForPID(pid uint32) string {
	if pid == 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)

	var buf [windows.MAX_PATH]uint16
	n := uint32(len(buf))
	if err := queryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// bareExeName returns the filename without path or extension.
func bareExeName(fullPath string) string {
	if fullPath == "" {
		return ""
	}
	name := filepath.Base(fullPath)
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// friendlyName returns a user-facing display name for a process.
// It reads the FileDescription from the executable's version-info resource
// (the same source Windows Volume Mixer uses), falling back to capitalised exe name.
func friendlyName(exePath, bareName string) string {
	if bareName == "" {
		return "Unknown"
	}

	// Try to read the FileDescription from the executable's version info.
	if exePath != "" {
		if desc := fileDescription(exePath); desc != "" {
			return desc
		}
	}

	// Fallback: capitalise the first letter of the bare exe name.
	if len(bareName) > 0 {
		return strings.ToUpper(bareName[:1]) + bareName[1:]
	}
	return bareName
}

var modVersion = windows.NewLazySystemDLL("version.dll")
var procGetFileVersionInfoSizeW = modVersion.NewProc("GetFileVersionInfoSizeW")
var procGetFileVersionInfoW = modVersion.NewProc("GetFileVersionInfoW")
var procVerQueryValueW = modVersion.NewProc("VerQueryValueW")

// fileDescription extracts the FileDescription string from an executable's
// embedded version-info resource.  Returns "" on any failure.
func fileDescription(exePath string) string {
	pathPtr, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return ""
	}

	// 1. Get the size of the version-info block.
	var dummy uint32
	size, _, _ := procGetFileVersionInfoSizeW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&dummy)),
	)
	if size == 0 {
		return ""
	}

	// 2. Read the version-info block into a buffer.
	data := make([]byte, size)
	r1, _, _ := procGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		size,
		uintptr(unsafe.Pointer(&data[0])),
	)
	if r1 == 0 {
		return ""
	}

	// 3. Query for \\VarFileInfo\\Translation to get the lang+codepage pair.
	subBlock, _ := syscall.UTF16PtrFromString(`\VarFileInfo\Translation`)
	var transPtr uintptr
	var transLen uint32
	r1, _, _ = procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(subBlock)),
		uintptr(unsafe.Pointer(&transPtr)),
		uintptr(unsafe.Pointer(&transLen)),
	)
	if r1 == 0 || transLen < 4 {
		return ""
	}

	// Translation table is an array of {uint16 lang, uint16 codepage}.
	lang := *(*uint16)(unsafe.Pointer(transPtr))
	cp := *(*uint16)(unsafe.Pointer(transPtr + 2))

	// 4. Query FileDescription using the first translation.
	descPath := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, lang, cp)
	descBlock, _ := syscall.UTF16PtrFromString(descPath)
	var descPtr uintptr
	var descLen uint32
	r1, _, _ = procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(descBlock)),
		uintptr(unsafe.Pointer(&descPtr)),
		uintptr(unsafe.Pointer(&descLen)),
	)
	if r1 == 0 || descLen == 0 {
		return ""
	}

	// descPtr points to a null-terminated UTF-16 string.
	desc := syscall.UTF16ToString((*[1024]uint16)(unsafe.Pointer(descPtr))[:descLen:descLen])
	return strings.TrimSpace(desc)
}

var modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
var procQueryFullProcessImageName = modKernel32.NewProc("QueryFullProcessImageNameW")

func queryFullProcessImageName(process windows.Handle, flags uint32, exeName *uint16, size *uint32) error {
	r1, _, e1 := procQueryFullProcessImageName.Call(
		uintptr(process),
		uintptr(flags),
		uintptr(unsafe.Pointer(exeName)),
		uintptr(unsafe.Pointer(size)),
	)
	if r1 == 0 {
		return e1
	}
	return nil
}
