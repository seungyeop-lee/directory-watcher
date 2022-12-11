package domain

import "time"

type Millisecond uint

func (m Millisecond) Duration() time.Duration {
	return time.Duration(m) * time.Millisecond
}
