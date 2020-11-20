package helper

import (
	"os"
)

func IsNotExist(path string) bool {
	return !IsExist(path)
}

func IsExist(path string) bool {
	_, err := os.Open(path)
	return err == nil
}

func IsDir(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.IsDir()
}
