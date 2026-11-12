package httpapi

import (
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	dto "github.com/prometheus/client_model/go"
	"sre_tbank/internal/metrics"
	"sre_tbank/internal/service"
)

func TestHTTPMetrics(t *testing.T) {
	m := metrics.New()
	a := &API{Service: &service.Service{Metrics: m}, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	r := gin.New()
	r.Use(a.httpMetrics(), a.recovery())
	gauge := func() float64 {
		t.Helper()
		families, err := m.Registry.Gather()
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range families {
			if f.GetName() == "support_http_requests_in_flight" {
				return f.Metric[0].GetGauge().GetValue()
			}
		}
		t.Fatal("missing in-flight gauge")
		return 0
	}
	r.GET("/api/v1/tickets/:id", func(c *gin.Context) {
		if got := gauge(); got != 1 {
			t.Errorf("in flight during handler = %v", got)
		}
		c.Status(200)
	})
	r.POST("/api/v1/tickets", func(c *gin.Context) { c.AbortWithStatus(401) })
	r.GET("/api/v1/panic", func(c *gin.Context) { panic("test") })
	r.GET("/debug/error", func(c *gin.Context) { c.Status(500) })
	for _, p := range []string{"/healthz", "/readyz", "/metrics", "/assets/app.js"} {
		r.GET(p, func(c *gin.Context) { c.Status(200) })
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/v1/tickets/123?secret=private", 200},
		{"GET", "/api/v1/tickets/456", 200},
		{"POST", "/api/v1/tickets", 401},
		{"GET", "/api/v1/panic", 500},
		{"GET", "/debug/error", 500},
		{"GET", "/api/v1/missing/123", 404},
		{"GET", "/api/v1/missing/456", 404},
		{"UNUSUAL", "/api/v1/missing", 404},
		{"GET", "/healthz", 200}, {"GET", "/readyz", 200},
		{"GET", "/metrics", 200}, {"GET", "/assets/app.js", 200},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
		if got := gauge(); got != 0 {
			t.Fatalf("in flight after request = %v", got)
		}
	}
	want := map[string]float64{
		"GET /api/v1/tickets/:id 200": 2, "POST /api/v1/tickets 401": 1,
		"GET /api/v1/panic 500": 1, "GET /debug/error 500": 1,
		"GET unmatched 404": 2, "OTHER unmatched 404": 1,
	}
	families, err := m.Registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	labels := func(metric *dto.Metric) map[string]string {
		result := map[string]string{}
		for _, label := range metric.Label {
			result[label.GetName()] = label.GetValue()
		}
		return result
	}
	var observations uint64
	for _, f := range families {
		switch f.GetName() {
		case "support_http_requests_total":
			for _, metric := range f.Metric {
				l := labels(metric)
				key := l["method"] + " " + l["route"] + " " + l["status"]
				expected, ok := want[key]
				if !ok || metric.GetCounter().GetValue() != expected {
					t.Errorf("unexpected counter %s = %v", key, metric.GetCounter().GetValue())
				}
				delete(want, key)
			}
		case "support_http_request_duration_seconds":
			for _, metric := range f.Metric {
				observations += metric.GetHistogram().GetSampleCount()
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing counters: %v", want)
	}
	if observations != 8 {
		t.Errorf("duration observations = %d, want 8", observations)
	}
}

func TestHTTPMetricsWithoutRegistry(t *testing.T) {
	r := gin.New()
	r.Use((&API{}).httpMetrics())
	r.GET("/api/test", func(c *gin.Context) { c.Status(204) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/test", nil))
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
}
