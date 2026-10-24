package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
)

func TestConfigurationFailureIsJSON(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	previousLogger, previousStdout := slog.Default(), os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer func() { os.Stdout = previousStdout; slog.SetDefault(previousLogger) }()
	os.Stdout = w
	runErr := run()
	_ = w.Close()
	data, err := io.ReadAll(r)
	if err != nil || runErr == nil {
		t.Fatalf("read: %v, run: %v", err, runErr)
	}
	var event map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
		t.Fatalf("startup output is not one JSON event: %s: %v", data, err)
	}
	if event["level"] != "ERROR" || event["message"] != "invalid configuration" || event["service.name"] != "support-desk" || event["timestamp"] == nil {
		t.Fatalf("invalid startup event: %v", event)
	}
}
