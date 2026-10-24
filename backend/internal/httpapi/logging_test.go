package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"sre_tbank/internal/config"
	"sre_tbank/internal/logging"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAccessLogSeverityAndErrorCorrelation(t *testing.T) {
	for _, tc := range []struct {
		status int
		level  string
	}{{200, "INFO"}, {401, "WARN"}, {500, "ERROR"}} {
		t.Run(tc.level, func(t *testing.T) {
			var out bytes.Buffer
			a := &API{Logger: logging.New(&out, slog.LevelInfo, "test", "test", "serve"), Config: config.Config{QueryTimeout: time.Second}}
			r := gin.New()
			r.Use(a.middleware())
			r.GET("/test", func(c *gin.Context) {
				if tc.status == 500 {
					a.fail(c, errors.New("private database credentials"))
					return
				}
				c.Status(tc.status)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/test?token=private-query", nil))
			if strings.Contains(out.String(), "private") {
				t.Fatal("sensitive request or error data leaked")
			}
			accessCount := 0
			scanner := bufio.NewScanner(&out)
			for scanner.Scan() {
				var event map[string]any
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					t.Fatal(err)
				}
				if event["request_id"] == "" || event["request_id"] != w.Header().Get("X-Request-ID") {
					t.Fatalf("missing request correlation: %v", event)
				}
				if event["message"] == "request" {
					accessCount++
					if event["level"] != tc.level || event["status"] != float64(tc.status) {
						t.Fatalf("incorrect access event: %v", event)
					}
				}
			}
			if accessCount != 1 {
				t.Fatalf("got %d access events, want 1", accessCount)
			}
		})
	}
}
