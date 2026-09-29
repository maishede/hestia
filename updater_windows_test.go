//go:build windows

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Hestia.exe")
	stage := filepath.Join(dir, "Hestia-v0.3.7.exe")
	backup := filepath.Join(dir, "previous.exe")
	oldBinary := []byte("MZ-old")
	newBinary := []byte("MZ-new")
	if err := os.WriteFile(target, oldBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage, newBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := replaceExecutable(stage, target, backup, func(path string) error {
		called = path == target
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, newBinary) || !called {
		t.Fatalf("new executable = %q, launch called = %v, error = %v", got, called, err)
	}
	previous, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(previous, oldBinary) {
		t.Fatalf("backup = %q, %v", previous, err)
	}
}

func TestReplaceExecutableRollsBackOnLaunchFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Hestia.exe")
	stage := filepath.Join(dir, "Hestia-v0.3.7.exe")
	backup := filepath.Join(dir, "previous.exe")
	oldBinary := []byte("MZ-old")
	if err := os.WriteFile(target, oldBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stage, []byte("MZ-new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceExecutable(stage, target, backup, func(string) error { return errors.New("blocked") }); err == nil {
		t.Fatal("failed launch should be reported")
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, oldBinary) {
		t.Fatalf("old executable was not restored: %q, %v", got, err)
	}
}
