package main

import (
	"fmt"
	"log"
	"os/exec"
	"soundshift/fyneTheme"
	"syscall"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"golang.org/x/sys/windows"
)

var Version string = "1.00"
var App fyne.App = app.NewWithID("SoundShift")
var Win fyne.Window = App.NewWindow("SoundShift")
var mainView = container.NewCenter()

func main() {
	//Win.SetIcon(fyne.NewStaticResource("icon", iconImg))
	Win.Resize(fyne.NewSize(200, 300))
	Win.CenterOnScreen()
	Win.SetTitle("SoundShift")
	Win.SetContent(mainView)
	App.Settings().SetTheme(fyneTheme.CustomTheme{})
	Win.SetFixedSize(true)
	Win.SetFullScreen(false)
	Win.SetMaster()

	Win.ShowAndRun()
}

func init() {
	Win.Hide()

	// width := int(win.GetSystemMetrics(win.SM_CXSCREEN))
	// height := int(win.GetSystemMetrics(win.SM_CYSCREEN))
	// moveWindow("SoundShift", width-215, height-345-GetTaskbarHeight(), 200, 300

}

func moveWindow(title string, x, y, width, height int) {
	windowHandle := getWindowHandleByName(title)

	psCommand := fmt.Sprintf(`
	Add-Type -TypeDefinition @"
	using System;
	using System.Runtime.InteropServices;
	public class PInvoke {
		[DllImport("user32.dll")]
		[return: MarshalAs(UnmanagedType.Bool)]
		public static extern bool GetWindowRect(IntPtr hwnd, out RECT lpRect);

		[DllImport("user32.dll")]
		[return: MarshalAs(UnmanagedType.Bool)]
		public static extern bool MoveWindow(IntPtr hwnd, int x, int y, int nWidth, int nHeight, bool repaint);

		[StructLayout(LayoutKind.Sequential)]
		public struct RECT {
			public int Left;
			public int Top;
			public int Right;
			public int Bottom;
		}
	}
"@

	$windowHandle = %[1]d

	$rect = New-Object PInvoke+RECT
	[void][PInvoke]::GetWindowRect($windowHandle, [ref]$rect)

	$windowWidth = %d
	$windowHeight = %d

	$posX = %2d
	$posY = %2d

	$result = [PInvoke]::MoveWindow($windowHandle, $posX, $posY, $windowWidth, $windowHeight, $true)
	`, windowHandle, width, height, x, y)

	cmd := exec.Command("powershell", "-Command", psCommand)
	err := cmd.Run()

	if err != nil {
		log.Fatalf("Failed to move window: %s", err)
	}
}

func getWindowHandleByName(title string) uintptr {
	user32dll := windows.MustLoadDLL("user32.dll")
	enumwindows := user32dll.MustFindProc("EnumWindows")

	var handle uintptr

	wndenumproc_function := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		var filename_data [100]uint16
		max_chars := uintptr(100)

		getwindowtextw := user32dll.MustFindProc("GetWindowTextW")
		getwindowtextw.Call(hwnd, uintptr(unsafe.Pointer(&filename_data)), max_chars)

		window_title := windows.UTF16ToString([]uint16(filename_data[:]))

		if window_title == title {
			handle = hwnd
			return 0
		}

		return 1
	})

	enumwindows.Call(wndenumproc_function, uintptr(0))
	return handle
}

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
