// Package metrics owns a process-local registry. Business events are recorded
// by the service only after the corresponding database operation commits.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry           *prometheus.Registry
	tickets            *prometheus.CounterVec
	ticketOpenAttempts *prometheus.CounterVec
	actions            *prometheus.CounterVec
	actionAttempts     *prometheus.CounterVec
	comments           *prometheus.CounterVec
	commentAttempts    *prometheus.CounterVec
	logins             *prometheus.CounterVec
	resolution         *prometheus.HistogramVec
	httpRequests       *prometheus.CounterVec
	httpDuration       *prometheus.HistogramVec
	httpInFlight       prometheus.Gauge
}

func New() *Metrics {
	m := &Metrics{
		Registry: prometheus.NewRegistry(),
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_http_requests_total", Help: "Completed API and lab debug HTTP requests by route template and response status.",
		}, []string{"method", "route", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "support_http_request_duration_seconds", Help: "API and lab debug HTTP handler duration, including failed requests.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		httpInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "support_http_requests_in_flight", Help: "API and lab debug HTTP requests currently being handled.",
		}),
		tickets: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_tickets_opened_total", Help: "Successfully persisted new tickets by initial priority.",
		}, []string{"priority"}),
		ticketOpenAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_ticket_open_attempts_total", Help: "Eligible attempts to create tickets by initial priority.",
		}, []string{"priority"}),
		actions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_ticket_actions_total", Help: "Successfully committed ticket workflow actions.",
		}, []string{"action"}),
		actionAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_ticket_action_attempts_total", Help: "Eligible attempts to perform ticket workflow actions.",
		}, []string{"action"}),
		comments: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_comments_added_total", Help: "Successfully committed explicit comments, excluding workflow messages.",
		}, []string{"author_role"}),
		commentAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "support_comment_add_attempts_total", Help: "Eligible attempts to add explicit comments by author role.",
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
		m.tickets, m.ticketOpenAttempts, m.actions, m.actionAttempts, m.comments, m.commentAttempts, m.logins, m.resolution,
		m.httpRequests, m.httpDuration, m.httpInFlight)
	for _, priority := range []string{"low", "medium", "high", "urgent"} {
		m.tickets.WithLabelValues(priority)
		m.ticketOpenAttempts.WithLabelValues(priority)
		m.resolution.WithLabelValues(priority)
	}
	for _, action := range []string{"claim", "reassign", "resolve", "confirm", "request_changes", "reopen"} {
		m.actions.WithLabelValues(action)
		m.actionAttempts.WithLabelValues(action)
	}
	for _, role := range []string{"user", "operator"} {
		m.comments.WithLabelValues(role)
		m.commentAttempts.WithLabelValues(role)
		m.logins.WithLabelValues(role)
	}
	return m
}

// StartHTTPRequest returns a completion callback, called after the response and
// panic recovery. Callers supply a route template, never the request URL.
func (m *Metrics) StartHTTPRequest(method, route string) func(int) {
	if m == nil {
		return func(int) {}
	}
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
	default:
		method = "OTHER"
	}
	start := time.Now()
	m.httpInFlight.Inc()
	return func(status int) {
		m.httpInFlight.Dec()
		m.httpRequests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
		m.httpDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
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

func (m *Metrics) TicketOpenAttempt(priority string) {
	if m != nil {
		m.ticketOpenAttempts.WithLabelValues(priority).Inc()
	}
}

func (m *Metrics) TicketAction(action string) {
	if m != nil {
		m.actions.WithLabelValues(action).Inc()
	}
}

func (m *Metrics) TicketActionAttempt(action string) {
	if m != nil {
		m.actionAttempts.WithLabelValues(action).Inc()
	}
}

func (m *Metrics) CommentAdded(role string) {
	if m != nil {
		m.comments.WithLabelValues(role).Inc()
	}
}

func (m *Metrics) CommentAddAttempt(role string) {
	if m != nil {
		m.commentAttempts.WithLabelValues(role).Inc()
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
