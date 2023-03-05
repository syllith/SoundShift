package general

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"soundshift/file"
	"soundshift/request"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/inancgumus/screen"
	"github.com/mitchellh/go-ps"
	"github.com/pterm/pterm"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
	"golang.org/x/sys/windows"
)

func Idle() {
	for {
		time.Sleep(500 * time.Millisecond)
	}
}

func ClearScreen() {
	screen.Clear()
	screen.MoveTopLeft()
}

func PrintHeader(title string) {
	pterm.DefaultHeader.
		WithBackgroundStyle(pterm.NewStyle(pterm.BgCyan)).
		WithTextStyle(pterm.NewStyle(pterm.FgBlack)).
		WithFullWidth().
		Println(title)
}

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

func GenUnixTime() int64 {
	return time.Now().UnixMilli()
	// res := request.Get("http://worldtimeapi.org/api/timezone/America/Chicago")

	// val, err := jsonparser.GetInt([]byte(res), "unixtime")
	// if err != nil {
	// 	return time.Now().UnixMilli()
	// }

	// return val * 1000
}

func MsToUnixTime(tm int64) time.Time {
	sec := tm / 1000
	msec := tm % 1000
	return time.Unix(sec, msec*int64(time.Millisecond))
}

func IsStringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func DeleteFromSlice(slice []string, stringToDelete string) []string {
	for i, v := range slice {
		if v == stringToDelete {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func StringToFloat(value string) float64 {
	convertedString, _ := strconv.ParseFloat(value, 64)
	return convertedString
}

func FloatToString(num float64) string {
	s := fmt.Sprintf("%f", num)
	return s
}

func CurrencyToFloat(input string) float64 {
	input = strings.ReplaceAll(input, "$", "")
	input = strings.ReplaceAll(input, ",", "")
	output := StringToFloat(input)
	return output
}

func Round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func ToFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(Round(num*output)) / output
}

func MinBetweenDates(startTime, stopTime int64) float64 {
	startDate := (float64(startTime) / 60000)
	endDate := (float64(stopTime) / 60000)
	return (endDate - startDate)
}

func GC() {
	for {
		time.Sleep(5 * time.Minute)
		runtime.GC()
	}
}

func ShuffleSlice(slice []string) {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	for n := len(slice); n > 0; n-- {
		randIndex := r.Intn(n)
		slice[n-1], slice[randIndex] = slice[randIndex], slice[n-1]
	}
}

func TrimString(input string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(input)), " ")
}

func KillProcByName(procname string) int {
	kill := exec.Command("taskkill", "/im", procname, "/T", "/F")
	err := kill.Run()
	if err != nil {
		//fmt.Println(err)
		return -1
	}
	return 0
}

func GetMemUsage() float64 {
	v, _ := mem.VirtualMemory()
	return v.UsedPercent
}

func GetCpuUsage() float64 {
	v, _ := cpu.Percent(0, true)
	return AverageSlice(v)
}

func AverageSlice(floatArr []float64) float64 {
	var total float64 = 0
	for _, value := range floatArr {
		total += value
	}
	return (total / float64(len(floatArr)))
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

func CreateRecord(data interface{}) {
	if request.Post("http://helios.wwt.com/app/api/mongowrite", &data) != "success" {
		if !file.Exists(file.RoamingDir() + "/helios/records.json") {
			file.CreateEmptyFile(file.RoamingDir() + "/helios/records.json")
		}
		recordFile, _ := os.Open(file.RoamingDir() + "/helios/records.json")

		var storedRecords []interface{}
		json.NewDecoder(recordFile).Decode(&storedRecords)

		storedRecords = append(storedRecords, data)
		j, _ := json.MarshalIndent(storedRecords, "", "  ")
		os.WriteFile(file.RoamingDir()+"/helios/records.json", j, fs.ModeAppend)
	}
}

func AlphaScrub(input string) string {
	regExp := regexp.MustCompile(`[\d\.]+`)
	matches := regExp.FindAllString(input, -1)
	newStr := strings.Join(matches, "")
	return newStr
}

func ContainsOnlySignedNumbers(str string) bool {
	hasNegative := false
	hasPositive := false
	hasDigits := false
	for i, r := range str {
		if r == '-' {
			if i != 0 { // negative sign can only appear at the beginning of the string
				return false
			}
			hasNegative = true
		} else if r == '+' {
			if i != 0 { // positive sign can only appear at the beginning of the string
				return false
			}
			hasPositive = true
		} else if !unicode.IsDigit(r) {
			return false // non-digit character found
		} else {
			hasDigits = true
		}
	}
	return hasDigits && (!hasNegative || !hasPositive)
}

func IsRunningAsAdmin() bool {
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	return err == nil
}

func RestartAsAdmin() {
	verb := "runas"
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()
	args := strings.Join(os.Args[1:], " ")

	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
	argPtr, _ := syscall.UTF16PtrFromString(args)

	var showCmd int32 = 1 //SW_NORMAL

	windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)

	os.Exit(0)

	// cmd := exec.Command("powershell", "-Command", "Start-Process", "-Verb", "runAs", os.Args[0])
	// cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	// cmd.Run()
	// os.Exit(0)
}
