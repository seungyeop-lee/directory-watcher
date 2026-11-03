package helper

import (
	"os"
)

func IsNotExist(path string) bool {
	return !IsExist(path)
}

func IsExist(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	return true
}

func IsDir(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.IsDir()
}
