package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsExposition(t *testing.T) {
	m := New()
	for _, accept := range []string{"text/plain; version=0.0.4", "application/openmetrics-text; version=1.0.0"} {
		r := httptest.NewRequest("GET", "/metrics", nil)
		r.Header.Set("Accept", accept)
		w := httptest.NewRecorder()
		m.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("metrics: %d", w.Code)
		}
		for _, name := range []string{"go_goroutines", "process_cpu_seconds_total", "support_tickets_opened_total", "support_ticket_actions_total", "support_comments_added_total", "support_logins_total", "support_ticket_resolution_duration_seconds_bucket"} {
			if !strings.Contains(w.Body.String(), name) {
				t.Fatalf("missing %s", name)
			}
		}
		if strings.Contains(accept, "openmetrics") && (!strings.HasPrefix(w.Header().Get("Content-Type"), "application/openmetrics-text") || !strings.HasSuffix(w.Body.String(), "# EOF\n")) {
			t.Fatal("OpenMetrics negotiation failed")
		}
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/", 404}, {"POST", "/metrics", 405}} {
		w := httptest.NewRecorder()
		m.Handler().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
	// Registries can coexist without global duplicate-collector registration.
	New()
}
