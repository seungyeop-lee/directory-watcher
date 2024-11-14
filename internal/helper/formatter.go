package helper

import (
	"fmt"
	"io"
)

const FormatterKey = "formatter"

type Formatter struct {
	prefix string
	colorF ColorFunc
	w      io.Writer
}

var _ io.Writer = (*Formatter)(nil)

func NewFormatter(prefix string, colorF ColorFunc, w io.Writer) *Formatter {
	return &Formatter{
		prefix: prefix,
		colorF: colorF,
		w:      w,
	}
}

func (f Formatter) Write(p []byte) (n int, err error) {
	return fmt.Fprintf(f.w, "%-20s%s%s", f.colorF(f.prefix), f.colorF(" | "), string(p))
}
