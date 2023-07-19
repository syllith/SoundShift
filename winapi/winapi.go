package winapi

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Constants
const (
	WS_MINIMIZEBOX  = 0x00020000
	WS_CAPTION      = 0x00C00000
	MF_BYCOMMAND    = 0x00000000
	SC_CLOSE        = 0xF060
	MF_GRAYED       = 0x00000001
	SM_CYSCREEN     = 1
	SPI_GETWORKAREA = 48
)

var (
	GWL_STYLE int32 = -16
)

type RECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// DisableMinMaxButtons disables the minimize button of the window with the specified title.
func DisableMinMaxButtons(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getWindowLong := user32.MustFindProc("GetWindowLongPtrW")
	setWindowLong := user32.MustFindProc("SetWindowLongPtrW")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		fmt.Printf("Failed to convert string to UTF16: %v\n", err)
		return
	}

	hwnd, _, err := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		fmt.Printf("Failed to get window handle: %v\n", err)
		return
	}

	style, _, err := getWindowLong.Call(hwnd, uintptr(GWL_STYLE))
	if style == 0 {
		fmt.Printf("Failed to get window style: %v\n", err)
		return
	}

	newStyle := style &^ uintptr(WS_MINIMIZEBOX)
	result, _, err := setWindowLong.Call(hwnd, uintptr(GWL_STYLE), newStyle)
	if result == 0 {
		fmt.Printf("Failed to set window style: %v\n", err)
	}
}

// DisableCloseButton disables the close button of the window with the specified title.
func DisableCloseButton(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getSystemMenu := user32.MustFindProc("GetSystemMenu")
	enableMenuItem := user32.MustFindProc("EnableMenuItem")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		fmt.Printf("Failed to convert string to UTF16: %v\n", err)
		return
	}

	hwnd, _, err := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		fmt.Printf("Failed to get window handle: %v\n", err)
		return
	}

	hMenu, _, err := getSystemMenu.Call(hwnd, uintptr(0))
	if hMenu == 0 {
		fmt.Printf("Failed to get system menu: %v\n", err)
		return
	}

	result, _, err := enableMenuItem.Call(hMenu, uintptr(SC_CLOSE), uintptr(MF_BYCOMMAND|MF_GRAYED))
	if result == 0xFFFFFFFF {
		fmt.Printf("Failed to disable close button: %v\n", err)
	}
}

// HideTitleBar hides the title bar of the window with the specified title.
func HideTitleBar(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getWindowLong := user32.MustFindProc("GetWindowLongPtrW")
	setWindowLong := user32.MustFindProc("SetWindowLongPtrW")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		fmt.Printf("Failed to convert string to UTF16: %v\n", err)
		return
	}

	hwnd, _, err := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		fmt.Printf("Failed to get window handle: %v\n", err)
		return
	}

	style, _, err := getWindowLong.Call(hwnd, uintptr(GWL_STYLE))
	if style == 0 {
		fmt.Printf("Failed to get window style: %v\n", err)
		return
	}

	newStyle := style &^ uintptr(WS_CAPTION)
	result, _, err := setWindowLong.Call(hwnd, uintptr(GWL_STYLE), newStyle)
	if result == 0 {
		fmt.Printf("Failed to set window style: %v\n", err)
	}
}

// MoveWindow moves the window with the specified title to the specified position and size.
func MoveWindow(title string, x, y, width, height int32) {
	user32 := syscall.MustLoadDLL("user32.dll")
	getWindowHandleByName := user32.MustFindProc("FindWindowW")
	moveWindow := user32.MustFindProc("MoveWindow")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		fmt.Printf("Failed to convert string to UTF16: %v\n", err)
		return
	}

	hwnd, _, err := getWindowHandleByName.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		fmt.Printf("Failed to get window handle: %s\n", err)
		return
	}

	result, _, err := moveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(width), uintptr(height), uintptr(1))
	if result == 0 {
		fmt.Printf("Failed to move window: %s\n", err)
	}
}

// GetTaskbarHeight returns the height of the taskbar.
func GetTaskbarHeight() int {
	user32 := windows.MustLoadDLL("user32.dll")
	getSystemMetrics := user32.MustFindProc("GetSystemMetrics")

	screenHeightRaw, _, _ := getSystemMetrics.Call(SM_CYSCREEN)
	screenHeight := int32(screenHeightRaw)

	rect := &RECT{}
	systemparametersinfo := user32.MustFindProc("SystemParametersInfoW")
	r1, _, err := systemparametersinfo.Call(SPI_GETWORKAREA, 0, uintptr(unsafe.Pointer(rect)), 0)

	if r1 != 1 {
		panic("SystemParametersInfoW failed: " + err.Error())
	}

	workAreaHeight := rect.Bottom
	taskbarHeight := screenHeight - workAreaHeight

	return int(taskbarHeight)
}
