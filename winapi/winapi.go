package winapi

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

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

type RECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

// . Convert int to uintptr due to inline conversion being impossible with negative numbers
func IntToUintptr(value int) uintptr {
	return uintptr(value)
}

// . Disables the minimize and maximize buttons
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

	style, _, err := getWindowLong.Call(hwnd, IntToUintptr(GWL_STYLE))
	if style == 0 {
		fmt.Printf("Failed to get window style: %v\n", err)
		return
	}

	newStyle := style &^ uintptr(WS_MINIMIZEBOX)
	result, _, err := setWindowLong.Call(hwnd, IntToUintptr(GWL_STYLE), newStyle)
	if result == 0 {
		fmt.Printf("Failed to set window style: %v\n", err)
	}
}

// . Disables the close button
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

// . Hide title bar
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

	style, _, err := getWindowLong.Call(hwnd, IntToUintptr(GWL_STYLE))
	if style == 0 {
		fmt.Printf("Failed to get window style: %v\n", err)
		return
	}

	newStyle := style &^ uintptr(WS_CAPTION)
	result, _, err := setWindowLong.Call(hwnd, IntToUintptr(GWL_STYLE), newStyle)
	if result == 0 {
		fmt.Printf("Failed to set window style: %v\n", err)
	}
}

// . Move window
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

// . Hide from taskbar
func HideWindowFromTaskbar(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getWindowLong := user32.MustFindProc("GetWindowLongW")
	setWindowLong := user32.MustFindProc("SetWindowLongW")

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

	// Use int32 for GWL_EXSTYLE
	gwl_exstyle := int32(-20)
	exStyle, _, err := getWindowLong.Call(hwnd, uintptr(gwl_exstyle))
	if exStyle == 0 {
		fmt.Printf("Failed to get window style: %v\n", err)
		return
	}

	newExStyle := (exStyle | uintptr(WS_EX_TOOLWINDOW)) &^ uintptr(WS_EX_APPWINDOW)
	result, _, err := setWindowLong.Call(hwnd, uintptr(gwl_exstyle), newExStyle)
	if result == 0 {
		fmt.Printf("Failed to set window style: %v\n", err)
	}
}

// . Show / Hide window
func ShowWindow(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	showWindow := user32.MustFindProc("ShowWindow")

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

	_, _, err = showWindow.Call(hwnd, uintptr(SW_SHOW))
	if err != nil {
		fmt.Printf("Failed to show window: %v\n", err)
	}
}

func HideWindow(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	showWindow := user32.MustFindProc("ShowWindow")

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

	_, _, err = showWindow.Call(hwnd, uintptr(SW_HIDE))
	if err != nil {
		fmt.Printf("Failed to hide window: %v\n", err)
	}
}

// . Topmost
func SetWindowAlwaysOnTop(title string) error {
	user32dll := windows.MustLoadDLL("user32.dll")
	findWindow := user32dll.MustFindProc("FindWindowW")
	setwindowpos := user32dll.MustFindProc("SetWindowPos")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return err
	}

	hwnd, _, err := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return fmt.Errorf("Failed to get window handle: %v", err)
	}

	_, _, err = setwindowpos.Call(hwnd, IntToUintptr(-1), 0, 0, 0, 0, SWP_NOSIZE|SWP_NOMOVE)
	if err != nil && err != syscall.Errno(0) {
		return err
	}

	return nil
}

func RemoveWindowAlwaysOnTop(title string) error {
	user32dll := windows.MustLoadDLL("user32.dll")
	findWindow := user32dll.MustFindProc("FindWindowW")
	setwindowpos := user32dll.MustFindProc("SetWindowPos")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return err
	}

	hwnd, _, err := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return fmt.Errorf("Failed to get window handle: %v", err)
	}

	_, _, err = setwindowpos.Call(hwnd, IntToUintptr(-2), 0, 0, 0, 0, SWP_NOSIZE|SWP_NOMOVE)
	if err != nil && err != syscall.Errno(0) {
		return err
	}

	return nil
}
