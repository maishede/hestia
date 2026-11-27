package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateDoesNotApplySettingsAfterSaveFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"port":8080,"precacheCovers":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	manager, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	setLanguage := func(c *Config) { c.Language = "ja" }
	if err := manager.Update(setLanguage); err == nil {
		t.Fatal("expected write failure")
	}
	read := make(chan Config, 1)
	go func() { read <- manager.Get() }()
	select {
	case after := <-read:
		if after.Language != "" {
			t.Fatalf("failed save modified config: %+v", after)
		}
	case <-time.After(time.Second):
		t.Fatal("reading config blocked after a write failure")
	}
	if err := os.Remove(path + ".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Update(setLanguage); err != nil || manager.Get().Language != "ja" {
		t.Fatalf("retry did not store the settings: %v", err)
	}
}
