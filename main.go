package main

import (
	"fmt"
	"soundshift/fyneTheme"
	"syscall"
	"time"
	"unsafe"

	"github.com/energye/systray"
	"github.com/energye/systray/icon"
	"github.com/lxn/win"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"golang.org/x/sys/windows"
)

var Version string = "1.00"
var App fyne.App = app.NewWithID("SoundShift")
var Win fyne.Window = App.NewWindow("SoundShift")
var mainView = container.NewCenter()

var screenWidth = int(win.GetSystemMetrics(win.SM_CXSCREEN))
var screenHeight = int(win.GetSystemMetrics(win.SM_CYSCREEN))
var taskbarHeight = GetTaskbarHeight()

func main() {
	//Win.SetIcon(fyne.NewStaticResource("icon", iconImg))
	App.Settings().SetTheme(fyneTheme.CustomTheme{})
	Win.Resize(fyne.NewSize(200, 300))
	Win.SetTitle("SoundShift")
	Win.SetContent(mainView)
	Win.SetFixedSize(true)
	Win.SetFullScreen(false)
	Win.SetMaster()
	go systray.Run(onReady, func() {})
	go func() {
		for {
			time.Sleep(250 * time.Millisecond)
			hideTitleBar("SoundShift")
			moveWindow("SoundShift", int32(screenWidth-215), int32(screenHeight-345-taskbarHeight), 200, 300)
		}
	}()

	Win.ShowAndRun()
}

func onReady() {
	systray.SetIcon(icon.Data)
	systray.SetTitle("SoudShift")
	systray.SetTooltip("SoudShift")
	systray.SetOnClick(func() {
		// Win.Show()
		// time.Sleep(100 * time.Millisecond)
		// moveWindow("SoundShift", screenWidth-215, screenHeight-345-taskbarHeight, 200, 300)
	})
}

// . Hide title bar

const (
	WS_CAPTION = 0x00C00000
)

var (
	GWL_STYLE int32 = -16
)

func hideTitleBar(title string) {
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

// . Move window
func moveWindow(title string, x, y, width, height int32) {
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

// func moveWindow(title string, x, y, width, height int) {
// 	windowHandle := getWindowHandleByName(title)
// 	fmt.Println(windowHandle)

// 	psCommand := fmt.Sprintf(`
// 	Add-Type -TypeDefinition @"
// 	using System;
// 	using System.Runtime.InteropServices;
// 	public class PInvoke {
// 		[DllImport("user32.dll")]
// 		[return: MarshalAs(UnmanagedType.Bool)]
// 		public static extern bool GetWindowRect(IntPtr hwnd, out RECT lpRect);

// 		[DllImport("user32.dll")]
// 		[return: MarshalAs(UnmanagedType.Bool)]
// 		public static extern bool MoveWindow(IntPtr hwnd, int x, int y, int nWidth, int nHeight, bool repaint);

// 		[StructLayout(LayoutKind.Sequential)]
// 		public struct RECT {
// 			public int Left;
// 			public int Top;
// 			public int Right;
// 			public int Bottom;
// 		}
// 	}
// "@

// 	$windowHandle = %[1]d

// 	$rect = New-Object PInvoke+RECT
// 	[void][PInvoke]::GetWindowRect($windowHandle, [ref]$rect)

// 	$windowWidth = %d
// 	$windowHeight = %d

// 	$posX = %2d
// 	$posY = %2d

// 	$result = [PInvoke]::MoveWindow($windowHandle, $posX, $posY, $windowWidth, $windowHeight, $true)
// 	`, windowHandle, width, height, x, y)

// 	cmd := exec.Command("powershell", "-Command", psCommand)
// 	err := cmd.Run()

// 	if err != nil {
// 		log.Fatalf("Failed to move window: %s", err)
// 	}
// }

// func getWindowHandleByName(title string) uintptr {
// 	user32dll := windows.MustLoadDLL("user32.dll")
// 	enumwindows := user32dll.MustFindProc("EnumWindows")

// 	var handle uintptr

// 	wndenumproc_function := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
// 		var filename_data [100]uint16
// 		max_chars := uintptr(100)

// 		getwindowtextw := user32dll.MustFindProc("GetWindowTextW")
// 		getwindowtextw.Call(hwnd, uintptr(unsafe.Pointer(&filename_data)), max_chars)

// 		window_title := windows.UTF16ToString([]uint16(filename_data[:]))

// 		if window_title == title {
// 			handle = hwnd
// 			return 0
// 		}

// 		return 1
// 	})

// 	enumwindows.Call(wndenumproc_function, uintptr(0))
// 	return handle
// }

type RECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

const (
	SM_CYSCREEN     = 1
	SPI_GETWORKAREA = 48
)

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

// audioDevices := mmDeviceEnumerator.GetDevices()

// for _, device := range audioDevices {
// 	fmt.Printf("Name: %s\nID: %s\n\n", device.Name, device.Id)
// }

//policyConfig.SetDefaultEndPoint("{0.0.0.00000000}.{5afc1da4-c1fa-4a96-bd5a-8de8cdc3563d}")
