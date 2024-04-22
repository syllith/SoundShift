package file

import (
	"errors"
	"os"
)

func Exists(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	return true
}

func RoamingDir() string {
	roaming, _ := os.UserConfigDir()
	return roaming
}

func Cwd() string {
	cwd, _ := os.Getwd()
	return cwd
}
