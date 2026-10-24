package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

func TestJSONSchemaAndLevel(t *testing.T) {
	var out bytes.Buffer
	level := new(slog.LevelVar)
	logger := New(&out, level, "lab-test", "minikube", "serve")
	logger.Info("request", "status", 200, "duration_ms", 12)
	var event map[string]any
	if err := json.Unmarshal(out.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{"level": "INFO", "message": "request", "service.name": "support-desk", "service.version": "lab-test", "deployment.environment.name": "minikube", "command": "serve", "status": float64(200), "duration_ms": float64(12)} {
		if event[k] != want {
			t.Errorf("%s: got %v, want %v", k, event[k], want)
		}
	}
	ts, err := time.Parse(time.RFC3339Nano, event["timestamp"].(string))
	if err != nil || ts.Location() != time.UTC {
		t.Fatalf("timestamp is not UTC RFC3339: %v", event["timestamp"])
	}
	out.Reset()
	level.Set(slog.LevelWarn)
	logger.Info("suppressed")
	if out.Len() != 0 {
		t.Fatal("configured level was ignored")
	}
}
