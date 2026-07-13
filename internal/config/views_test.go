package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestViews(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	views, err := LoadViews()
	if err != nil {
		t.Fatalf("LoadViews() failed: %v", err)
	}
	if len(views) != 0 {
		t.Errorf("expected empty views, got %d", len(views))
	}

	if err := SaveView("test-view", "rating>=4"); err != nil {
		t.Fatalf("SaveView() failed: %v", err)
	}

	views, err = LoadViews()
	if err != nil {
		t.Fatalf("LoadViews() after save failed: %v", err)
	}
	if len(views) != 1 {
		t.Errorf("expected 1 view, got %d", len(views))
	}
	if views["test-view"] != "rating>=4" {
		t.Errorf("expected 'rating>=4', got %q", views["test-view"])
	}

	if err := SaveView("another", "status=reading"); err != nil {
		t.Fatalf("SaveView() second failed: %v", err)
	}

	views, err = LoadViews()
	if err != nil {
		t.Fatalf("LoadViews() after second save failed: %v", err)
	}
	if len(views) != 2 {
		t.Errorf("expected 2 views, got %d", len(views))
	}

	if err := DeleteView("test-view"); err != nil {
		t.Fatalf("DeleteView() failed: %v", err)
	}

	views, err = LoadViews()
	if err != nil {
		t.Fatalf("LoadViews() after delete failed: %v", err)
	}
	if len(views) != 1 {
		t.Errorf("expected 1 view after delete, got %d", len(views))
	}
	if _, ok := views["test-view"]; ok {
		t.Error("test-view should be deleted")
	}

	if err := DeleteView("nonexistent"); err == nil {
		t.Error("DeleteView() should fail for nonexistent view")
	}

	viewsPath := filepath.Join(tmpDir, viewsFileName)
	if _, err := os.Stat(viewsPath); os.IsNotExist(err) {
		t.Error("views file should exist")
	}
}
