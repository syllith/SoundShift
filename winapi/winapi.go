package winapi

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows constants for window styles, show commands, and other properties
const (
	WS_MINIMIZEBOX   = 0x00020000
	WS_CAPTION       = 0x00C00000
	MF_BYCOMMAND     = 0x00000000
	SC_CLOSE         = 0xF060
	MF_GRAYED        = 0x00000001
	SM_CYSCREEN      = 1
	SPI_GETWORKAREA  = 48
	GWL_STYLE        = -16
	WS_EX_TOOLWINDOW = 0x00000080
	WS_EX_APPWINDOW  = 0x00040000
	SW_HIDE          = 0
	SW_SHOW          = 5
	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
)

// RECT defines a rectangle area used by Windows APIs for window positioning
type RECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// . IntToUintptr converts an integer to uintptr, needed for negative constants in Windows API calls
func IntToUintptr(value int) uintptr {
	return uintptr(value)
}

// . GetHwnd finds a window handle (HWND) based on a process ID and window title
func GetHwnd(pid uint32, title string) (windows.HWND, error) {
	//* Load required functions from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	enumWindows := user32.MustFindProc("EnumWindows")
	getWindowThreadProcessId := user32.MustFindProc("GetWindowThreadProcessId")
	getWindowTextW := user32.MustFindProc("GetWindowTextW")
	isWindowVisible := user32.MustFindProc("IsWindowVisible")

	var hwnd windows.HWND
	found := false

	//* Define the callback function for EnumWindows
	cb := syscall.NewCallback(func(h windows.HWND, lparam uintptr) uintptr {
		var windowPid uint32
		//* Retrieve the process ID associated with the current window handle
		getWindowThreadProcessId.Call(uintptr(h), uintptr(unsafe.Pointer(&windowPid)))
		if windowPid == pid {
			//* Get the window title text
			buf := make([]uint16, 256)
			getWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			windowTitle := syscall.UTF16ToString(buf)

			//* Check if the window is visible
			ret, _, _ := isWindowVisible.Call(uintptr(h))
			if windowTitle == title && ret != 0 {
				//* Window matches the title and is visible; set hwnd and stop enumeration
				hwnd = h
				found = true
				return 0 // Stop enumeration
			}
		}
		return 1 // Continue enumeration
	})

	//* Call EnumWindows with the defined callback to enumerate all windows
	r1, _, err := enumWindows.Call(cb, 0)
	if r1 == 0 && !found {
		//! Window with specified title and PID not found
		return 0, errors.New("window not found")
	}

	//* Handle potential errors from EnumWindows
	if err != nil && err.Error() != "The operation completed successfully." {
		return 0, err
	}
	return hwnd, nil
}

// . HideMinMaxButtons removes the minimize and maximize buttons from the window's title bar
func HideMinMaxButtons(hwnd windows.HWND) {
	//* Load necessary functions from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	getWindowLong := user32.MustFindProc("GetWindowLongPtrW")
	setWindowLong := user32.MustFindProc("SetWindowLongPtrW")

	//* Retrieve the current window style
	style, _, _ := getWindowLong.Call(uintptr(hwnd), IntToUintptr(GWL_STYLE))
	if style == 0 {
		return // Return if unable to retrieve the style
	}

	//* Modify the style to remove the minimize box
	newStyle := style &^ uintptr(WS_MINIMIZEBOX)
	_, _, _ = setWindowLong.Call(uintptr(hwnd), IntToUintptr(GWL_STYLE), newStyle)
}

// . DisableCloseButton disables the close button in the window's title bar
func DisableCloseButton(hwnd windows.HWND) {
	//* Load necessary functions from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	getSystemMenu := user32.MustFindProc("GetSystemMenu")
	enableMenuItem := user32.MustFindProc("EnableMenuItem")

	//* Retrieve the system menu handle for the window
	hMenu, _, _ := getSystemMenu.Call(uintptr(hwnd), uintptr(0))
	if hMenu == 0 {
		return // Return if unable to retrieve the system menu
	}

	//* Disable the close button in the system menu
	_, _, _ = enableMenuItem.Call(hMenu, uintptr(SC_CLOSE), uintptr(MF_BYCOMMAND|MF_GRAYED))
}

// . HideTitleBar removes the title bar from the window
func HideTitleBar(hwnd windows.HWND) {
	//* Load necessary functions from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	getWindowLong := user32.MustFindProc("GetWindowLongPtrW")
	setWindowLong := user32.MustFindProc("SetWindowLongPtrW")

	//* Retrieve the current window style
	style, _, _ := getWindowLong.Call(uintptr(hwnd), IntToUintptr(GWL_STYLE))
	if style == 0 {
		return // Return if unable to retrieve the style
	}

	//* Modify the style to remove the title bar
	newStyle := style &^ uintptr(WS_CAPTION)
	_, _, _ = setWindowLong.Call(uintptr(hwnd), IntToUintptr(GWL_STYLE), newStyle)
}

// . MoveWindow relocates and resizes the specified window to the given position and dimensions
func MoveWindow(hwnd windows.HWND, x, y, width, height int32) {
	//* Load the MoveWindow function from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	moveWindow := user32.MustFindProc("MoveWindow")

	//* Call MoveWindow to change the position and size of the window
	_, _, _ = moveWindow.Call(uintptr(hwnd), uintptr(x), uintptr(y), uintptr(width), uintptr(height), uintptr(1))
}

// . GetTaskbarHeight returns the current height of the Windows taskbar
func GetTaskbarHeight() int {
	//* Load necessary functions from user32.dll
	user32 := windows.MustLoadDLL("user32.dll")
	getSystemMetrics := user32.MustFindProc("GetSystemMetrics")

	//* Get the screen height in pixels
	screenHeightRaw, _, _ := getSystemMetrics.Call(SM_CYSCREEN)
	screenHeight := int32(screenHeightRaw)

	//* Retrieve the work area (screen area excluding the taskbar)
	rect := &RECT{}
	systemparametersinfo := user32.MustFindProc("SystemParametersInfoW")
	r1, _, err := systemparametersinfo.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(rect)), 0)
	if r1 != 1 {
		panic("SystemParametersInfoW failed: " + err.Error())
	}

	//* Calculate the taskbar height as the difference between screen height and work area height
	workAreaHeight := rect.Bottom
	taskbarHeight := screenHeight - workAreaHeight

	return int(taskbarHeight)
}

// . HideWindowFromTaskbar removes the window from the Windows taskbar by adjusting its extended window styles
func HideWindowFromTaskbar(hwnd windows.HWND) {
	//* Load necessary functions from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	getWindowLong := user32.MustFindProc("GetWindowLongW")
	setWindowLong := user32.MustFindProc("SetWindowLongW")

	//* Retrieve the current extended window style
	gwl_exstyle := int32(-20)
	exStyle, _, _ := getWindowLong.Call(uintptr(hwnd), uintptr(gwl_exstyle))
	if exStyle == 0 {
		return // Return if unable to retrieve the extended style
	}

	//* Modify the style to hide the window from the taskbar
	newExStyle := (exStyle | uintptr(WS_EX_TOOLWINDOW)) &^ uintptr(WS_EX_APPWINDOW)
	_, _, _ = setWindowLong.Call(uintptr(hwnd), uintptr(gwl_exstyle), newExStyle)
}

// . ShowWindow makes the specified window visible on the screen
func ShowWindow(hwnd windows.HWND) {
	//* Load the ShowWindow function from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	showWindow := user32.MustFindProc("ShowWindow")

	//* Call ShowWindow to display the window
	_, _, _ = showWindow.Call(uintptr(hwnd), uintptr(SW_SHOW))
}

// . HideWindow hides the specified window by calling the ShowWindow function with SW_HIDE
func HideWindow(hwnd windows.HWND) {
	//* Load the ShowWindow function from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	showWindow := user32.MustFindProc("ShowWindow")

	//* Call ShowWindow with SW_HIDE to hide the window
	_, _, _ = showWindow.Call(uintptr(hwnd), uintptr(SW_HIDE))
}

// . IsWindowVisible checks if the specified window is currently visible
func IsWindowVisible(hwnd windows.HWND) bool {
	//* Load the IsWindowVisible function from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	isWindowVisible := user32.MustFindProc("IsWindowVisible")

	//* Call IsWindowVisible to check visibility status
	ret, _, _ := isWindowVisible.Call(uintptr(hwnd))
	return ret != 0 // Returns true if the window is visible
}

// . SetTopmost sets the specified window to be always on top (topmost)
func SetTopmost(hwnd windows.HWND) {
	//* Load the SetWindowPos function from user32.dll
	user32dll := windows.MustLoadDLL("user32.dll")
	setwindowpos := user32dll.MustFindProc("SetWindowPos")

	//* Call SetWindowPos with HWND_TOPMOST to make the window topmost
	_, _, _ = setwindowpos.Call(uintptr(hwnd), IntToUintptr(-1), 0, 0, 0, 0, SWP_NOSIZE|SWP_NOMOVE)
}

// . RemoveTopmost removes the always-on-top status from the specified window
func RemoveTopmost(hwnd windows.HWND) {
	//* Load the SetWindowPos function from user32.dll
	user32dll := windows.MustLoadDLL("user32.dll")
	setwindowpos := user32dll.MustFindProc("SetWindowPos")

	//* Call SetWindowPos with HWND_NOTOPMOST to remove the topmost property
	_, _, _ = setwindowpos.Call(uintptr(hwnd), IntToUintptr(-2), 0, 0, 0, 0, SWP_NOSIZE|SWP_NOMOVE)
}

// . GetWindowPosition retrieves the current position of the specified window on the screen
func GetWindowPosition(hwnd windows.HWND) (x, y int32, err error) {
	//* Load the GetWindowRect function from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	getWindowRect := user32.MustFindProc("GetWindowRect")

	//* Retrieve the window's RECT structure containing its screen coordinates
	var rect RECT
	r1, _, err := getWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	if r1 == 0 {
		return 0, 0, err // Return an error if the call fails
	}

	//* Extract the top-left corner coordinates of the window
	return rect.Left, rect.Top, nil
}

// . GetWindowSize retrieves the dimensions (width and height) of the specified window
func GetWindowSize(hwnd windows.HWND) (width, height int32, err error) {
	//* Load the GetWindowRect function from user32.dll
	user32 := syscall.MustLoadDLL("user32.dll")
	getWindowRect := user32.MustFindProc("GetWindowRect")

	//* Retrieve the window's RECT structure to calculate its size
	var rect RECT
	r1, _, err := getWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	if r1 == 0 {
		return 0, 0, err // Return an error if the call fails
	}

	//* Calculate width and height from RECT coordinates
	width = rect.Right - rect.Left
	height = rect.Bottom - rect.Top
	return width, height, nil
}
