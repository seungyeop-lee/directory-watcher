package harness

import (
	"reflect"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestReceivedChangesFollowWatcherOrder(t *testing.T) {
	paths := map[string]bool{"last.txt": true, "normal-19.txt": true}
	log := "[DEBUG] | event: " + (fsnotify.Event{Name: "last.txt", Op: fsnotify.Create}).String() + "\n" +
		"[DEBUG] | event: " + (fsnotify.Event{Name: "readiness.probe", Op: fsnotify.Create}).String() + "\n" +
		"[DEBUG] | event: " + (fsnotify.Event{Name: "normal-19.txt", Op: fsnotify.Create}).String() + "\n" +
		"[DEBUG] | event: " + (fsnotify.Event{Name: "normal-19.txt", Op: fsnotify.Write}).String() + "\n" +
		"[DEBUG] | event: " + (fsnotify.Event{Name: "normal-19.txt", Op: fsnotify.Chmod}).String() + "\n"
	for _, tc := range []struct {
		name string
		ops  fsnotify.Op
		want []string
	}{
		{"create", fsnotify.Create, []string{"last.txt", "normal-19.txt"}},
		{"create-write", fsnotify.Create | fsnotify.Write, []string{"last.txt", "normal-19.txt", "normal-19.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := receivedChanges(log, paths, tc.ops); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("received changes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestChangesCompletedUsesReceivedBoundary(t *testing.T) {
	for _, tc := range []struct {
		name       string
		received   []string
		records    []string
		sequential bool
		want       bool
	}{
		{"no-receipt", nil, []string{"DONE:last.txt"}, false, false},
		{"no-completion", []string{"normal-19.txt"}, nil, false, false},
		{"last-created-is-not-last-received", []string{"last.txt", "normal-19.txt"}, []string{"DONE:normal-19.txt"}, false, true},
		{"earlier-completion-is-insufficient", []string{"last.txt", "normal-19.txt"}, []string{"DONE:last.txt"}, false, false},
		{"repeated-path-still-running", []string{"shared.txt", "shared.txt"}, []string{"BEGIN|1|DONE:shared.txt", "DONE:shared.txt", "BEGIN|2|DONE:shared.txt"}, false, false},
		{"cancelled-then-completed", []string{"last.txt", "normal-19.txt"}, []string{"BEGIN|1|DONE:last.txt", "CANCEL|1|DONE:last.txt", "BEGIN|2|DONE:normal-19.txt", "DONE:normal-19.txt"}, false, true},
		{"sequential-must-drain-received-duplicates", []string{"shared.txt", "shared.txt"}, []string{"BEGIN|1|DONE:shared.txt", "DONE:shared.txt"}, true, false},
		{"sequential-drained", []string{"shared.txt", "shared.txt"}, []string{"BEGIN|1|DONE:shared.txt", "DONE:shared.txt", "BEGIN|2|DONE:shared.txt", "DONE:shared.txt"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := changesCompleted(tc.received, tc.records, "DONE:", tc.sequential); got != tc.want {
				t.Fatalf("completed = %t, want %t; received=%v records=%v", got, tc.want, tc.received, tc.records)
			}
		})
	}
}
