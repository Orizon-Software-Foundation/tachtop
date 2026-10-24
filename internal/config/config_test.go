package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Theme != "tachtop" || cfg.RefreshMS != DefaultRefreshMS || cfg.TempUnit != "c" {
		t.Fatalf("Default() = %+v", cfg)
	}
}

func TestLoadMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg, gotPath, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing: %v", err)
	}
	if gotPath != path {
		t.Errorf("path = %q, want %q", gotPath, path)
	}
	if cfg != Default() {
		t.Errorf("missing file should give defaults, got %+v", cfg)
	}
}

func TestLoadValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"theme":"nord","refresh_ms":200,"temp_unit":"F"}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "nord" {
		t.Errorf("Theme = %q", cfg.Theme)
	}
	if cfg.RefreshMS != MinRefreshMS {
		t.Errorf("RefreshMS = %d, want clamped %d", cfg.RefreshMS, MinRefreshMS)
	}
	if cfg.TempUnit != "f" {
		t.Errorf("TempUnit = %q, want f", cfg.TempUnit)
	}
}

func TestLoadNormalizesEmptyValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"theme":"","refresh_ms":90000,"temp_unit":"kelvin"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "tachtop" {
		t.Errorf("Theme = %q", cfg.Theme)
	}
	if cfg.RefreshMS != MaxRefreshMS {
		t.Errorf("RefreshMS = %d, want %d", cfg.RefreshMS, MaxRefreshMS)
	}
	if cfg.TempUnit != "c" {
		t.Errorf("TempUnit = %q, want c", cfg.TempUnit)
	}
}

func TestLoadMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
	if cfg != Default() {
		t.Errorf("malformed config should fall back to defaults, got %+v", cfg)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"theme":"nord","refesh_ms":500}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err == nil {
		t.Fatal("expected an error for a misspelled field")
	}
	if cfg != Default() {
		t.Errorf("unknown field should fall back to defaults, got %+v", cfg)
	}
}

func TestLoadTrimsTheme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"theme":"  nord  "}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Theme != "nord" {
		t.Errorf("Theme = %q, want nord", cfg.Theme)
	}
}

func TestPathHonoursEnv(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom.json")
	t.Setenv("TACHTOP_CONFIG", want)
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestPathUsesConfigHome(t *testing.T) {
	t.Setenv("TACHTOP_CONFIG", "")
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if got, want := Path(), filepath.Join(dir, "tachtop", "config.json"); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}
