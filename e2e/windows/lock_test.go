//go:build darwin

package main

import (
	"errors"
	"testing"
)

func TestLockVMSharesTestsAndExcludesStartAndStop(t *testing.T) {
	uuid := "lock-test-" + t.Name()
	first, err := lockVM(uuid, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := lockVM(uuid, false)
	if err != nil {
		t.Fatalf("second test run was refused: %v", err)
	}
	if _, err := lockVM(uuid, true); !errors.Is(err, errVMBusy) {
		t.Fatalf("stop during test runs = %v, want errVMBusy", err)
	}
	first.Close()
	second.Close()
	stop, err := lockVM(uuid, true)
	if err != nil {
		t.Fatalf("stop after test runs ended: %v", err)
	}
	defer stop.Close()
	if _, err := lockVM(uuid, false); !errors.Is(err, errVMBusy) {
		t.Fatalf("test run during stop = %v, want errVMBusy", err)
	}
	other, err := lockVM("other-"+uuid, true)
	if err != nil {
		t.Fatalf("another VM was locked too: %v", err)
	}
	other.Close()
}
