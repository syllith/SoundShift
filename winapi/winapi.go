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

// . Cached DLL and proc handles — loaded once at startup instead of on every call
var (
	user32dll                    = windows.MustLoadDLL("user32.dll")
	procEnumWindows              = user32dll.MustFindProc("EnumWindows")
	procGetWindowThreadProcessId = user32dll.MustFindProc("GetWindowThreadProcessId")
	procGetWindowTextW           = user32dll.MustFindProc("GetWindowTextW")
	procIsWindowVisible          = user32dll.MustFindProc("IsWindowVisible")
	procGetWindowLongPtrW        = user32dll.MustFindProc("GetWindowLongPtrW")
	procSetWindowLongPtrW        = user32dll.MustFindProc("SetWindowLongPtrW")
	procGetWindowLongW           = user32dll.MustFindProc("GetWindowLongW")
	procSetWindowLongW           = user32dll.MustFindProc("SetWindowLongW")
	procGetSystemMenu            = user32dll.MustFindProc("GetSystemMenu")
	procEnableMenuItem           = user32dll.MustFindProc("EnableMenuItem")
	procMoveWindow               = user32dll.MustFindProc("MoveWindow")
	procSetWindowPos             = user32dll.MustFindProc("SetWindowPos")
	procShowWindow               = user32dll.MustFindProc("ShowWindow")
	procGetWindowRect            = user32dll.MustFindProc("GetWindowRect")
	procGetSystemMetrics         = user32dll.MustFindProc("GetSystemMetrics")
	procSystemParametersInfoW    = user32dll.MustFindProc("SystemParametersInfoW")
)

// . IntToUintptr converts an integer to uintptr, needed for negative constants in Windows API calls
func IntToUintptr(value int) uintptr {
	return uintptr(value)
}

// . GetHwnd finds a window handle (HWND) based on a process ID and window title
func GetHwnd(pid uint32, title string) (windows.HWND, error) {
	var hwnd windows.HWND
	found := false

	//* Define the callback function for EnumWindows
	cb := syscall.NewCallback(func(h windows.HWND, lparam uintptr) uintptr {
		var windowPid uint32
		//* Retrieve the process ID associated with the current window handle
		procGetWindowThreadProcessId.Call(uintptr(h), uintptr(unsafe.Pointer(&windowPid)))
		if windowPid == pid {
			//* Get the window title text
			buf := make([]uint16, 256)
			procGetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			windowTitle := syscall.UTF16ToString(buf)

			//* Check if the window is visible
			ret, _, _ := procIsWindowVisible.Call(uintptr(h))
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
	r1, _, err := procEnumWindows.Call(cb, 0)
	if r1 == 0 && !found {
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
	//* Retrieve the current window style
	style, _, _ := procGetWindowLongPtrW.Call(uintptr(hwnd), IntToUintptr(GWL_STYLE))
	if style == 0 {
		return
	}

	//* Modify the style to remove the minimize box
	newStyle := style &^ uintptr(WS_MINIMIZEBOX)
	procSetWindowLongPtrW.Call(uintptr(hwnd), IntToUintptr(GWL_STYLE), newStyle)
}

// . DisableCloseButton disables the close button in the window's title bar
func DisableCloseButton(hwnd windows.HWND) {
	//* Retrieve the system menu handle for the window
	hMenu, _, _ := procGetSystemMenu.Call(uintptr(hwnd), uintptr(0))
	if hMenu == 0 {
		return
	}

	//* Disable the close button in the system menu
	procEnableMenuItem.Call(hMenu, uintptr(SC_CLOSE), uintptr(MF_BYCOMMAND|MF_GRAYED))
}

// . HideTitleBar removes the title bar from the window
func HideTitleBar(hwnd windows.HWND) {
	//* Retrieve the current window style
	style, _, _ := procGetWindowLongPtrW.Call(uintptr(hwnd), IntToUintptr(GWL_STYLE))
	if style == 0 {
		return
	}

	//* Modify the style to remove the title bar
	newStyle := style &^ uintptr(WS_CAPTION)
	procSetWindowLongPtrW.Call(uintptr(hwnd), IntToUintptr(GWL_STYLE), newStyle)
}

// . MoveWindow relocates and resizes the specified window to the given position and dimensions
func MoveWindow(hwnd windows.HWND, x, y, width, height int32) {
	procMoveWindow.Call(uintptr(hwnd), uintptr(x), uintptr(y), uintptr(width), uintptr(height), uintptr(1))
}

// . GetTaskbarHeight returns the current height of the Windows taskbar
func GetTaskbarHeight() int {
	//* Get the screen height in pixels
	screenHeightRaw, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
	screenHeight := int32(screenHeightRaw)

	//* Retrieve the work area (screen area excluding the taskbar)
	rect := &RECT{}
	r1, _, err := procSystemParametersInfoW.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(rect)), 0)
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
	gwl_exstyle := int32(-20)

	//* Retrieve the current extended window style
	exStyle, _, _ := procGetWindowLongW.Call(uintptr(hwnd), uintptr(gwl_exstyle))
	if exStyle == 0 {
		return
	}

	//* Modify the style to hide the window from the taskbar
	newExStyle := (exStyle | uintptr(WS_EX_TOOLWINDOW)) &^ uintptr(WS_EX_APPWINDOW)
	procSetWindowLongW.Call(uintptr(hwnd), uintptr(gwl_exstyle), newExStyle)
}

// . ShowWindow makes the specified window visible on the screen
func ShowWindow(hwnd windows.HWND) {
	procShowWindow.Call(uintptr(hwnd), uintptr(SW_SHOW))
}

// . HideWindow hides the specified window by calling the ShowWindow function with SW_HIDE
func HideWindow(hwnd windows.HWND) {
	procShowWindow.Call(uintptr(hwnd), uintptr(SW_HIDE))
}

// . IsWindowVisible checks if the specified window is currently visible
func IsWindowVisible(hwnd windows.HWND) bool {
	ret, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	return ret != 0
}

// . SetTopmost sets the specified window to be always on top (topmost)
func SetTopmost(hwnd windows.HWND) {
	procSetWindowPos.Call(uintptr(hwnd), IntToUintptr(-1), 0, 0, 0, 0, SWP_NOSIZE|SWP_NOMOVE)
}

// . RemoveTopmost removes the always-on-top status from the specified window
func RemoveTopmost(hwnd windows.HWND) {
	procSetWindowPos.Call(uintptr(hwnd), IntToUintptr(-2), 0, 0, 0, 0, SWP_NOSIZE|SWP_NOMOVE)
}

// . GetWindowPosition retrieves the current position of the specified window on the screen
func GetWindowPosition(hwnd windows.HWND) (x, y int32, err error) {
	var rect RECT
	r1, _, err := procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	if r1 == 0 {
		return 0, 0, err
	}
	return rect.Left, rect.Top, nil
}

// . GetWindowSize retrieves the dimensions (width and height) of the specified window
func GetWindowSize(hwnd windows.HWND) (width, height int32, err error) {
	var rect RECT
	r1, _, err := procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	if r1 == 0 {
		return 0, 0, err
	}
	width = rect.Right - rect.Left
	height = rect.Bottom - rect.Top
	return width, height, nil
}
