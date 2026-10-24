package main

import "testing"

func TestRunVersion(t *testing.T) {
	if err := run([]string{"--version"}); err != nil {
		t.Fatalf("run --version: %v", err)
	}
}

func TestRunListThemes(t *testing.T) {
	if err := run([]string{"--list-themes"}); err != nil {
		t.Fatalf("run --list-themes: %v", err)
	}
}

func TestRunHelp(t *testing.T) {
	if err := run([]string{"--help"}); err != nil {
		t.Fatalf("run --help: %v", err)
	}
}

func TestRunUnknownFlag(t *testing.T) {
	if err := run([]string{"--definitely-not-a-flag"}); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
}

func TestRunUnexpectedArgument(t *testing.T) {
	if err := run([]string{"extra"}); err == nil {
		t.Fatal("expected an error for an unexpected argument")
	}
}
