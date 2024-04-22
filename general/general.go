package general

import (
	"fmt"
	"os"
	"path/filepath"
	"soundshift/file"
	"strings"
	"time"
	"unicode"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/mitchellh/go-ps"
)

func IsProcRunning(name string) bool {
	count := 0
	processList, err := ps.Processes()
	if err != nil {
		return false
	}

	for x := range processList {
		process := processList[x]
		processName := process.Executable()
		if strings.Contains(strings.ToLower(processName), strings.ToLower(name)) {
			count++
		}
	}
	return count > 1
}

func EllipticalTruncate(text string, maxLen int) string {
	lastSpaceIx := maxLen
	len := 0
	for i, r := range text {
		if unicode.IsSpace(r) {
			lastSpaceIx = i
		}
		len++
		if len > maxLen {
			return text[:lastSpaceIx] + "..."
		}
	}

	return text
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

func LogError(message string, err error) {
	// Construct the path to the log file using the roaming directory.
	logPath := filepath.Join(file.RoamingDir(), "soundshift/log.txt")

	// Open or create the log file for appending.
	f, fileErr := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if fileErr != nil {
		// If the log file itself cannot be opened, print the error to stderr.
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", fileErr)
		return
	}
	defer f.Close()

	// Format the current time, message, and error to a string.
	logEntry := fmt.Sprintf("%s: %s - %v\n", time.Now().Format(time.RFC3339), message, err)

	// Write the log entry to the file.
	if _, writeErr := f.WriteString(logEntry); writeErr != nil {
		// If writing to the file fails, print the error to stderr.
		fmt.Fprintf(os.Stderr, "Failed to write to log file: %v\n", writeErr)
	}
}
