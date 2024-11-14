package cmd

import (
	"fmt"
	"io"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type colorWriter struct {
	w     io.Writer
	color helper.ColorFunc
}

func newColorWriter(w io.Writer, color helper.ColorFunc) *colorWriter {
	return &colorWriter{
		w:     w,
		color: color,
	}
}

func (w *colorWriter) Write(p []byte) (n int, err error) {
	return fmt.Fprint(w.w, w.color(string(p)))
}
