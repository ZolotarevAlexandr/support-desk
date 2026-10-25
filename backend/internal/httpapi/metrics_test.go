package httpapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func (h *harness) metric(name, label, value string) *dto.Metric {
	h.t.Helper()
	families, err := h.a.Service.Metrics.Registry.Gather()
	if err != nil {
		h.t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			for _, pair := range metric.Label {
				if pair.GetName() == label && pair.GetValue() == value {
					return metric
				}
			}
		}
	}
	h.t.Fatalf("missing %s{%s=%q}", name, label, value)
	return nil
}

func (h *harness) count(name, label, value string, want float64) {
	h.t.Helper()
	if got := h.metric(name, label, value).GetCounter().GetValue(); got != want {
		h.t.Fatalf("%s{%s=%q}=%v, want %v", name, label, value, got, want)
	}
}

func TestBusinessMetrics(t *testing.T) {
	h := setup(t)
	_, ut := h.register("metrics-user", "user")
	_, ot := h.register("metrics-operator", "operator")
	h.count("support_logins_total", "role", "user", 0)
	h.request("POST", "/api/v1/auth/login", "", map[string]any{"email": "metrics-user@example.com", "password": "wrong"}, 401)
	h.request("POST", "/api/v1/auth/login", "", map[string]any{"email": "metrics-user@example.com", "password": "strong-password-123"}, 200)
	h.count("support_logins_total", "role", "user", 1)

	ticket := h.ticket(ut, "Metrics test")
	h.count("support_tickets_opened_total", "priority", "medium", 1)
	h.request("POST", "/api/v1/tickets", ut, map[string]any{}, 400)
	h.count("support_tickets_opened_total", "priority", "medium", 1)
	path := fmt.Sprintf("/api/v1/tickets/%d/comments", ticket.ID)
	h.request("POST", path, ut, map[string]any{"body": "An explicit comment"}, 201)
	h.request("POST", path, ot, map[string]any{"body": "An operator reply"}, 201)
	h.count("support_comments_added_total", "author_role", "user", 1)
	h.count("support_comments_added_total", "author_role", "operator", 1)
	h.action(ut, ticket.ID, map[string]any{"action": "claim"}, 403)
	h.action(ot, ticket.ID, map[string]any{"action": "claim"}, 200)
	h.action(ot, ticket.ID, map[string]any{"action": "claim"}, 409)
	h.count("support_ticket_actions_total", "action", "claim", 1)
	_, err := h.a.Service.Store.DB.ExecContext(context.Background(), "UPDATE tickets SET created_at=? WHERE id=?", time.Now().UTC().Add(-2*time.Hour), ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	h.action(ot, ticket.ID, map[string]any{"action": "resolve", "message": "Resolved"}, 200)
	h.action(ot, ticket.ID, map[string]any{"action": "resolve", "message": "Duplicate"}, 409)
	h.count("support_ticket_actions_total", "action", "resolve", 1)
	histogram := h.metric("support_ticket_resolution_duration_seconds", "priority", "medium").GetHistogram()
	if histogram.GetSampleCount() != 1 || histogram.GetSampleSum() < 7199 || histogram.GetSampleSum() > 7205 {
		t.Fatalf("resolution histogram: %v", histogram)
	}
	h.action(ut, ticket.ID, map[string]any{"action": "confirm"}, 200)
	h.action(ot, ticket.ID, map[string]any{"action": "reopen", "message": "Investigate again"}, 200)
	h.action(ot, ticket.ID, map[string]any{"action": "resolve", "message": "Resolved again"}, 200)
	if got := h.metric("support_ticket_resolution_duration_seconds", "priority", "medium").GetHistogram().GetSampleCount(); got != 2 {
		t.Fatalf("repeat resolution count %d", got)
	}
	h.count("support_comments_added_total", "author_role", "operator", 1)
	h.count("support_comments_added_total", "author_role", "user", 1)
	h.request("GET", "/metrics", "", nil, 404)
}
