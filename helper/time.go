package helper

import "time"

func CreateThreshold() <-chan time.Time {
	return time.After(time.Millisecond * 100)
}
