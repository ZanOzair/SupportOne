package main

import (
	"bytes"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestVersionFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if err := run([]string{"--version"}, env(nil), &stdout, &stderr); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout.String(), "supportone-server") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

// TestItRefusesToStartWithoutAToken is the whole reason this is the default:
// a fleet server with no token is a list of other people's machines, served
// to anyone who finds the port.
func TestItRefusesToStartWithoutAToken(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run([]string{"--data", t.TempDir()}, env(nil), &stdout, &stderr)
	if err == nil {
		t.Fatal("the server started with no token")
	}
	if !strings.Contains(err.Error(), "SUPPORTONE_FLEET_TOKEN") {
		t.Errorf("the error does not say what to set: %v", err)
	}
}

func TestAWhitespaceTokenIsNoToken(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run([]string{"--data", t.TempDir()}, env(map[string]string{TokenEnv: "   "}), &stdout, &stderr)
	if err == nil {
		t.Error("the server started with a whitespace token")
	}
}

func TestAShortTokenIsRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := run([]string{"--data", t.TempDir()}, env(map[string]string{TokenEnv: "short"}), &stdout, &stderr)
	if err == nil {
		t.Error("the server started with a guessable token")
	}
}

func TestUnexpectedArgumentsAreRefused(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if err := run([]string{"serve"}, env(nil), &stdout, &stderr); err == nil {
		t.Error("run accepted a positional argument")
	}
}
