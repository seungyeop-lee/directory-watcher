package domain

import "strings"

type FileName string

func (n FileName) IsDefaultExcludeFile() bool {
	return strings.HasPrefix(string(n), ".") ||
		strings.HasPrefix(string(n), "_")
}
