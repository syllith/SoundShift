package winapi

import (
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

// . Checks if the window handle exists with the given title
func WindowExists(title string) bool {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return false
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	return hwnd != 0
}

// . Hides the minimize / maximize buttons in the title bar
func HideMinMaxButtons(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getWindowLong := user32.MustFindProc("GetWindowLongPtrW")
	setWindowLong := user32.MustFindProc("SetWindowLongPtrW")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return
	}

	style, _, _ := getWindowLong.Call(hwnd, IntToUintptr(GWL_STYLE))
	if style == 0 {
		return
	}

	newStyle := style &^ uintptr(WS_MINIMIZEBOX)
	_, _, _ = setWindowLong.Call(hwnd, IntToUintptr(GWL_STYLE), newStyle)
}

// . Disables the close button in the title bar
func DisableCloseButton(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getSystemMenu := user32.MustFindProc("GetSystemMenu")
	enableMenuItem := user32.MustFindProc("EnableMenuItem")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return
	}

	hMenu, _, _ := getSystemMenu.Call(hwnd, uintptr(0))
	if hMenu == 0 {
		return
	}

	_, _, _ = enableMenuItem.Call(hMenu, uintptr(SC_CLOSE), uintptr(MF_BYCOMMAND|MF_GRAYED))
}

// . Hides the title bar
func HideTitleBar(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getWindowLong := user32.MustFindProc("GetWindowLongPtrW")
	setWindowLong := user32.MustFindProc("SetWindowLongPtrW")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return
	}

	style, _, _ := getWindowLong.Call(hwnd, IntToUintptr(GWL_STYLE))
	if style == 0 {
		return
	}

	newStyle := style &^ uintptr(WS_CAPTION)
	_, _, _ = setWindowLong.Call(hwnd, IntToUintptr(GWL_STYLE), newStyle)
}

// . Relocates and resizes the window
func MoveWindow(title string, x, y, width, height int32) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	moveWindow := user32.MustFindProc("MoveWindow")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return
	}

	_, _, _ = moveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(width), uintptr(height), uintptr(1))
}

// . Returns the taskbars current height
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

// . Hides the window from the task bar
func HideWindowFromTaskbar(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getWindowLong := user32.MustFindProc("GetWindowLongW")
	setWindowLong := user32.MustFindProc("SetWindowLongW")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return
	}

	gwl_exstyle := int32(-20)
	exStyle, _, _ := getWindowLong.Call(hwnd, uintptr(gwl_exstyle))
	if exStyle == 0 {
		return
	}

	newExStyle := (exStyle | uintptr(WS_EX_TOOLWINDOW)) &^ uintptr(WS_EX_APPWINDOW)
	_, _, _ = setWindowLong.Call(hwnd, uintptr(gwl_exstyle), newExStyle)
}

// . Shows the window
func ShowWindow(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	showWindow := user32.MustFindProc("ShowWindow")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return
	}

	_, _, _ = showWindow.Call(hwnd, uintptr(SW_SHOW))
}

// . Hides the window
func HideWindow(title string) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	showWindow := user32.MustFindProc("ShowWindow")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return
	}

	_, _, _ = showWindow.Call(hwnd, uintptr(SW_HIDE))
}

// . Checks if the window is visible
func IsWindowVisible(title string) bool {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	isWindowVisible := user32.MustFindProc("IsWindowVisible")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return false
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return false
	}

	ret, _, _ := isWindowVisible.Call(hwnd)
	return ret != 0
}

// . Sets the window to be topmost
func SetTopmost(title string) {
	user32dll := windows.MustLoadDLL("user32.dll")
	findWindow := user32dll.MustFindProc("FindWindowW")
	setwindowpos := user32dll.MustFindProc("SetWindowPos")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return
	}

	_, _, _ = setwindowpos.Call(hwnd, IntToUintptr(-1), 0, 0, 0, 0, SWP_NOSIZE|SWP_NOMOVE)
}

// . Removes window topmost
func RemoveTopmost(title string) {
	user32dll := windows.MustLoadDLL("user32.dll")
	findWindow := user32dll.MustFindProc("FindWindowW")
	setwindowpos := user32dll.MustFindProc("SetWindowPos")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return
	}

	_, _, _ = setwindowpos.Call(hwnd, IntToUintptr(-2), 0, 0, 0, 0, SWP_NOSIZE|SWP_NOMOVE)
}

func GetWindowPosition(title string) (x, y int32, err error) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getWindowRect := user32.MustFindProc("GetWindowRect")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0, 0, err
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return 0, 0, syscall.GetLastError()
	}

	var rect RECT
	r1, _, err := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if r1 == 0 {
		return 0, 0, err
	}

	return rect.Left, rect.Top, nil
}

func GetWindowSize(title string) (width, height int32, err error) {
	user32 := syscall.MustLoadDLL("user32.dll")
	findWindow := user32.MustFindProc("FindWindowW")
	getWindowRect := user32.MustFindProc("GetWindowRect")

	ptr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return 0, 0, err
	}

	hwnd, _, _ := findWindow.Call(uintptr(0), uintptr(unsafe.Pointer(ptr)))
	if hwnd == 0 {
		return 0, 0, syscall.GetLastError()
	}

	var rect RECT
	r1, _, err := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if r1 == 0 {
		return 0, 0, err
	}

	width = rect.Right - rect.Left
	height = rect.Bottom - rect.Top
	return width, height, nil
}
