// Package metrics owns a process-local registry. Business events are recorded
// by the service only after the corresponding database operation commits.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry   *prometheus.Registry
	tickets    *prometheus.CounterVec
	actions    *prometheus.CounterVec
	comments   *prometheus.CounterVec
	logins     *prometheus.CounterVec
	resolution *prometheus.HistogramVec
}

func New() *Metrics {
	m := &Metrics{
		Registry: prometheus.NewRegistry(),
		tickets: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_tickets_opened_total", Help: "Successfully persisted new tickets by initial priority.",
		}, []string{"priority"}),
		actions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_ticket_actions_total", Help: "Successfully committed ticket workflow actions.",
		}, []string{"action"}),
		comments: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_comments_added_total", Help: "Successfully committed explicit comments, excluding workflow messages.",
		}, []string{"author_role"}),
		logins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_logins_total", Help: "Successful explicit logins with a persisted session, excluding registration.",
		}, []string{"role"}),
		resolution: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "support_ticket_resolution_duration_seconds", Help: "Ticket age since original creation at each committed resolution, including repeated resolutions.",
			Buckets: []float64{1, 5, 15, 30, 60, 300, 900, 3600, 21600, 86400, 604800},
		}, []string{"priority"}),
	}
	m.Registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.tickets, m.actions, m.comments, m.logins, m.resolution)
	for _, priority := range []string{"low", "medium", "high", "urgent"} {
		m.tickets.WithLabelValues(priority)
		m.resolution.WithLabelValues(priority)
	}
	for _, action := range []string{"claim", "reassign", "resolve", "confirm", "request_changes", "reopen"} {
		m.actions.WithLabelValues(action)
	}
	for _, role := range []string{"user", "operator"} {
		m.comments.WithLabelValues(role)
		m.logins.WithLabelValues(role)
	}
	return m
}

func (m *Metrics) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	return mux
}

// Nil receivers let administrative callers and existing service tests omit metrics.
func (m *Metrics) TicketOpened(priority string) {
	if m != nil {
		m.tickets.WithLabelValues(priority).Inc()
	}
}

func (m *Metrics) TicketAction(action string) {
	if m != nil {
		m.actions.WithLabelValues(action).Inc()
	}
}

func (m *Metrics) CommentAdded(role string) {
	if m != nil {
		m.comments.WithLabelValues(role).Inc()
	}
}

func (m *Metrics) Login(role string) {
	if m != nil {
		m.logins.WithLabelValues(role).Inc()
	}
}

func (m *Metrics) TicketResolved(priority string, age time.Duration) {
	if m != nil {
		m.resolution.WithLabelValues(priority).Observe(max(0, age.Seconds()))
	}
}
