package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestErrorContextAndFailureExit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "records.log")
	cmd := exec.Command("go", "run", ".", "--file="+file, "--error-context", "--exit-code=7", "--line=NOTIFY")
	values := map[string]string{
		"DW_ERROR_TARGET":        "/tmp/a target;$(false)",
		"DW_ERROR_KIND":          "event_loss",
		"DW_ERROR_CAUSE":         "original 'cause' {{.Path}}",
		"DW_ERROR_MESSAGE":       "watch target failed: original cause",
		"DW_ERROR_POSSIBLE_LOSS": "true",
	}
	cmd.Env = os.Environ()
	for key, value := range values {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || !strings.Contains(string(out), "exit status 7") {
		t.Fatalf("configured failure exit missing: %v %s", err, out)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[1] != "NOTIFY" || !strings.HasPrefix(lines[0], "ERROR|") {
		t.Fatalf("records: %q", lines)
	}
	var context map[string]string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[0], "ERROR|")), &context); err != nil {
		t.Fatal(err)
	}
	for key, want := range values {
		if context[key] != want {
			t.Errorf("%s = %q, want %q", key, context[key], want)
		}
	}
	workdir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if context["workdir"] != workdir {
		t.Errorf("workdir = %q, want %q", context["workdir"], workdir)
	}
}

func TestQueueContext(t *testing.T) {
	file := filepath.Join(t.TempDir(), "records.log")
	cmd := exec.Command("go", "run", ".", "--file="+file, "--queue-context", "--line=WARN")
	values := map[string]string{
		"DW_QUEUE_TARGET":    "/tmp/a target",
		"DW_QUEUE_LEVEL":     "70",
		"DW_QUEUE_DIRECTION": "up",
		"DW_QUEUE_LENGTH":    "7",
		"DW_QUEUE_CAPACITY":  "10",
		"DW_QUEUE_TOP_PATHS": "2 /tmp/a target/a.txt\n1 /tmp/a target/b.txt",
	}
	cmd.Env = os.Environ()
	for key, value := range values {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hookrec failed: %v %s", err, out)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[1] != "WARN" || !strings.HasPrefix(lines[0], "QUEUE|") {
		t.Fatalf("records: %q", lines)
	}
	var context map[string]string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[0], "QUEUE|")), &context); err != nil {
		t.Fatal(err)
	}
	if len(context) != len(values) {
		t.Errorf("queue context fields = %v, want %v", context, values)
	}
	for key, want := range values {
		if context[key] != want {
			t.Errorf("%s = %q, want %q", key, context[key], want)
		}
	}
}
