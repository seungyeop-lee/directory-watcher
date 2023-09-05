package domain

import (
	"strings"
)

type PathSuffixes []PathSuffix

func (s PathSuffixes) Contain(input Path) bool {
	for _, ps := range s {
		if ps.Contain(input) {
			return true
		}
	}
	return false
}

type PathSuffix string

func (s PathSuffix) Contain(input Path) bool {
	return strings.HasSuffix(input.Abs(), string(s))
}
