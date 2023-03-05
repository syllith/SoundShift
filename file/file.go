package file

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/pterm/pterm"
)

func Read(file string) string {
	content, err := os.ReadFile(file)
	if err != nil {
		// fmt.Println(err)
		return ""
	}
	return string(content)
}

func Write(file string, content []byte) {
	err := os.WriteFile(file, content, 0644)
	if err != nil {
		//fmt.Println(err)
	}
}

func CreateEmptyDir(name string) {
	err := os.Mkdir(name, os.ModePerm)
	if err != nil {
		//fmt.Println(err)
	}
}

func Delete(location string) {
	err := os.Remove(location)
	if err != nil {
		//fmt.Println(err)
	}
}

func Exists(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func CreateEmptyFile(name string) {
	_, err := os.OpenFile(name, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		//fmt.Println(err)
	}
}

func Download(filepath string, url string) (err error) {
	//* file.Download("./Reignforest v1.2.exe", html.EscapeString("https://skydrive.digi-safe.co/files/Reignforest Repo/Reignforest v1.2.exe"))

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func ReadDir(path string) []string {
	files, err := os.ReadDir(path)
	if err != nil {
		//fmt.Println(err)
	}

	var fileList []string
	for _, file := range files {
		fileList = append(fileList, file.Name())
	}

	return fileList
}

func CreateShortcut(src, dst string) error {
	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED|ole.COINIT_SPEED_OVER_MEMORY)
	oleShellObject, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return err
	}
	defer oleShellObject.Release()
	wshell, err := oleShellObject.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer wshell.Release()
	cs, err := oleutil.CallMethod(wshell, "CreateShortcut", dst)
	if err != nil {
		return err
	}
	idispatch := cs.ToIDispatch()
	oleutil.PutProperty(idispatch, "TargetPath", src)
	oleutil.CallMethod(idispatch, "Save")
	return nil
}

func DocDir() string {
	path, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return path + "/Documents"
	} else {
		return path
	}
}

func RoamingDir() string {
	roaming, _ := os.UserConfigDir()
	return roaming
}

func Cwd() string {
	cwd, _ := os.Getwd()
	return cwd
}

func PrintDownloadPercent(done chan int64, path string, total int64) {
	var stop bool = false
	p, _ := pterm.DefaultProgressbar.WithRemoveWhenDone().WithTotal(100).Start()
	p.UpdateTitle("Downloading")

	for {
		select {
		case <-done:
			stop = true
		default:
			file, _ := os.Open(path)
			fi, _ := file.Stat()
			size := fi.Size()

			if size == 0 {
				size = 1
			}

			var percent float64 = float64(size) / float64(total) * 100

			p.Current = int(percent)
			p.Add(0)
		}

		if stop {
			p.Stop()
			break
		}

		time.Sleep(50 * time.Millisecond)
	}
}
