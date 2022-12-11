package helper

import "time"

func CreateThreshold(d time.Duration) <-chan time.Time {
	if d == 0 {
		d = time.Millisecond * 100
	}
	return time.After(d)
}
