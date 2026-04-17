package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	file := flag.String("file", "", "log file path")
	line := flag.String("line", "", "line to append")
	flag.Parse()

	if *file == "" {
		fmt.Fprintln(os.Stderr, "hookrec: --file is required")
		os.Exit(2)
	}

	f, err := os.OpenFile(*file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hookrec:", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, *line); err != nil {
		fmt.Fprintln(os.Stderr, "hookrec:", err)
		os.Exit(1)
	}
}
