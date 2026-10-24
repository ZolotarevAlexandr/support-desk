package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// New uses the same schema for the server and its administrative commands.
func New(w io.Writer, level slog.Leveler, version, environment, command string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 {
				switch a.Key {
				case slog.TimeKey:
					a.Key = "timestamp"
					a.Value = slog.TimeValue(a.Value.Time().UTC())
				case slog.MessageKey:
					a.Key = "message"
				}
			}
			return a
		},
	})).With("service.name", "support-desk", "service.version", version,
		"deployment.environment.name", environment, "command", command)
}

// Goose writes its migration progress through the application's JSON handler.
type Goose struct{ Logger *slog.Logger }

func (g Goose) Printf(format string, args ...interface{}) {
	g.Logger.Info(strings.TrimSpace(fmt.Sprintf(format, args...)))
}

func (g Goose) Fatalf(format string, args ...interface{}) {
	// Preserve Goose's fatal contract without exposing driver error payloads.
	g.Logger.Error("migration stopped")
	os.Exit(1)
}
