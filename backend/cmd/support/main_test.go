package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
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

func TestServeReleasesListenerOnSecondBindFailure(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := first.Addr().String()
	_ = first.Close()
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	if err := serve(context.Background(), time.Second, func() {}, &http.Server{Addr: address}, &http.Server{Addr: occupied.Addr().String()}); err == nil {
		t.Fatal("occupied metrics port accepted")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("business listener leaked: %v", err)
	}
	_ = listener.Close()
}

func TestServeCancellationClosesBothServers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopped := false
	if err := serve(ctx, time.Second, func() { stopped = true }, &http.Server{Addr: "127.0.0.1:0"}, &http.Server{Addr: "127.0.0.1:0"}); err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("readiness not cleared")
	}
}
