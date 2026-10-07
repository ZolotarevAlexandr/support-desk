package httpapi

import (
	"context"
	"errors"
	"fmt"
	"sre_tbank/internal/service"
	"sre_tbank/internal/store"
	"sync"
	"testing"
)

func (h *harness) action(token string, id int64, body any, status int) store.Ticket {
	h.t.Helper()
	w := h.request("POST", fmt.Sprintf("/api/v1/tickets/%d/actions", id), token, body, status)
	if status == 200 {
		return decode[store.Ticket](h.t, w)
	}

	return store.Ticket{}
}

func TestTicketActions(t *testing.T) {
	h := setup(t)
	u, ut := h.register("requester", "user")
	_, other := h.register("other", "user")
	op, ot := h.register("operator", "operator")
	op2, ot2 := h.register("second-operator", "operator")
	ticket := h.ticket(ut, "Workflow test")

	h.action(other, ticket.ID, map[string]any{"action": "claim"}, 404)
	h.action(ut, ticket.ID, map[string]any{"action": "claim"}, 403)
	h.action(ut, ticket.ID, map[string]any{"action": "confirm"}, 409)
	h.action(ot, ticket.ID, map[string]any{"action": "resolve", "message": "Too soon"}, 409)
	h.action(ot, ticket.ID, map[string]any{"action": "claim", "assignee_id": op2.ID}, 400)

	claimed := h.action(ot, ticket.ID, map[string]any{"action": "claim"}, 200)
	if claimed.AssigneeID == nil || *claimed.AssigneeID != op.ID || claimed.Status != "in_progress" || claimed.AssigneeName != op.Name || claimed.RequesterName != u.Name {
		t.Fatalf("claim did not atomically assign/start/project names: %+v", claimed)
	}

	visible := decode[store.Ticket](t, h.request("GET", fmt.Sprintf("/api/v1/tickets/%d", ticket.ID), ut, nil, 200))
	if visible.AssigneeName != op.Name {
		t.Fatal("requester cannot see assignee name")
	}

	h.action(ot2, ticket.ID, map[string]any{"action": "claim"}, 409)
	for _, body := range []map[string]any{{"action": "resolve"}, {"action": "resolve", "message": "  "}, {"action": "resolve", "message": nil}, {"action": "reassign"}, {"action": "nonsense"}, {"action": "claim", "requester_id": op.ID}} {
		h.action(ot, ticket.ID, body, 400)
	}

	h.action(ot, ticket.ID, map[string]any{"action": "reassign", "assignee_id": u.ID}, 400)

	reassigned := h.action(ot, ticket.ID, map[string]any{"action": "reassign", "assignee_id": op2.ID}, 200)
	if reassigned.AssigneeName != op2.Name {
		t.Fatal("assignment name not refreshed")
	}

	returned := h.action(ot2, ticket.ID, map[string]any{"action": "reassign", "assignee_id": nil}, 200)
	if returned.Status != "open" || returned.AssigneeID != nil || returned.AssigneeName != "" {
		t.Fatal("return to queue")
	}

	h.action(ot, ticket.ID, map[string]any{"action": "claim"}, 200)
	h.request("PATCH", fmt.Sprintf("/api/v1/tickets/%d", ticket.ID), ot, map[string]any{"status": "closed"}, 400)

	resolved := h.action(ot, ticket.ID, map[string]any{"action": "resolve", "message": "Resolution one"}, 200)
	if resolved.Status != "resolved" {
		t.Fatal("resolve")
	}

	h.action(ot2, ticket.ID, map[string]any{"action": "confirm"}, 403)

	h.action(other, ticket.ID, map[string]any{"action": "confirm"}, 404)
	h.action(ot, ticket.ID, map[string]any{"action": "reassign", "assignee_id": nil}, 409)
	h.action(ut, ticket.ID, map[string]any{"action": "request_changes", "message": "  "}, 400)

	feedback := h.action(ut, ticket.ID, map[string]any{"action": "request_changes", "message": "Still fails after restart"}, 200)
	if feedback.Status != "in_progress" || *feedback.AssigneeID != op.ID {
		t.Fatal("feedback lost assignee")
	}

	h.action(ut, ticket.ID, map[string]any{"action": "request_changes", "message": "Duplicate feedback"}, 409)
	h.action(ot, ticket.ID, map[string]any{"action": "resolve", "message": "Resolution two"}, 200)

	closed := h.action(ut, ticket.ID, map[string]any{"action": "confirm"}, 200)
	if closed.Status != "closed" {
		t.Fatal("confirmation did not close")
	}

	h.action(ut, ticket.ID, map[string]any{"action": "confirm"}, 409)
	h.action(ut, ticket.ID, map[string]any{"action": "reopen", "message": "Unauthorized reopening"}, 403)

	comments := decode[store.Page[store.Comment]](t, h.request("GET", fmt.Sprintf("/api/v1/tickets/%d/comments", ticket.ID), ut, nil, 200))
	if comments.Total != 4 || comments.Items[1].Body != "Still fails after restart" || comments.Items[1].AuthorID != u.ID {
		t.Fatalf("workflow messages: %+v", comments)
	}

	reopened := h.action(ot, ticket.ID, map[string]any{"action": "reopen", "message": "Further investigation needed"}, 200)
	if reopened.Status != "in_progress" {
		t.Fatal("reopening lost assignment")
	}

	h.action(ot, ticket.ID, map[string]any{"action": "reopen", "message": "Cannot reopen active work"}, 409)
}

func TestCompetingClaimsAndConfirmations(t *testing.T) {
	h := setup(t)
	u, ut := h.register("requester", "user")
	op, _ := h.register("operator", "operator")
	op2, _ := h.register("operator-two", "operator")
	ticket := h.ticket(ut, "Concurrent claim")

	ctx := context.Background()

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup

	for _, actor := range []store.User{op, op2} {
		wg.Add(1)
		go func(actor store.User) {
			defer wg.Done()
			<-start
			_, err := h.a.Service.TicketAction(ctx, actor, ticket.ID, service.TicketActionInput{Action: "claim"})
			results <- err
		}(actor)
	}

	close(start)
	wg.Wait()
	close(results)

	success, conflicts := 0, 0
	for err := range results {
		var e *service.Error
		if err == nil {
			success++
		} else if errors.As(err, &e) && e.Status == 409 {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}

	if success != 1 || conflicts != 1 {
		t.Fatalf("claims: %d successful, %d conflicts", success, conflicts)
	}

	message := "Resolved atomically"
	if _, err := h.a.Service.TicketAction(ctx, op, ticket.ID, service.TicketActionInput{Action: "resolve", Message: &message}); err != nil {
		t.Fatal(err)
	}

	start = make(chan struct{})
	results = make(chan error, 2)

	for _, action := range []string{"confirm", "request_changes"} {
		wg.Add(1)
		go func(action string) {
			defer wg.Done()
			<-start
			in := service.TicketActionInput{Action: action}
			if action == "request_changes" {
				in.Message = &message
			}

			_, err := h.a.Service.TicketAction(ctx, u, ticket.ID, in)
			results <- err
		}(action)
	}

	close(start)
	wg.Wait()
	close(results)

	success, conflicts = 0, 0
	for err := range results {
		var e *service.Error
		if err == nil {
			success++
		} else if errors.As(err, &e) && e.Status == 409 {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}

	if success != 1 || conflicts != 1 {
		t.Fatal("requester outcomes were not serialized")
	}

	comments, err := h.a.Service.Comments(ctx, u, ticket.ID, 1, 25)
	if err != nil || comments.Total != 2 {
		t.Fatalf("losing transition wrote a message: %+v %v", comments, err)
	}
}

func TestResolutionRollsBackMessageOnWriteFailure(t *testing.T) {
	h := setup(t)
	u, ut := h.register("requester", "user")
	op, ot := h.register("operator", "operator")

	ticket := h.ticket(ut, "Rollback test")
	h.action(ot, ticket.ID, map[string]any{"action": "claim"}, 200)

	ctx := context.Background()
	db := h.a.Service.Store.DB
	_, err := db.ExecContext(ctx, `CREATE OR REPLACE FUNCTION reject_test_resolution() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test write failure'; END; $$; CREATE TRIGGER reject_test_resolution BEFORE UPDATE ON tickets FOR EACH ROW WHEN (NEW.title='Rollback test' AND NEW.status='resolved') EXECUTE FUNCTION reject_test_resolution();`)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _, _ = db.ExecContext(ctx, "DROP FUNCTION IF EXISTS reject_test_resolution() CASCADE") })

	message := "Must not persist"
	if _, err := h.a.Service.TicketAction(ctx, op, ticket.ID, service.TicketActionInput{Action: "resolve", Message: &message}); err == nil {
		t.Fatal("write unexpectedly succeeded")
	}

	current, err := h.a.Service.Ticket(ctx, u, ticket.ID)
	if err != nil || current.Status != "in_progress" {
		t.Fatal("status did not roll back", err)
	}

	comments, err := h.a.Service.Comments(ctx, u, ticket.ID, 1, 25)
	if err != nil || comments.Total != 0 {
		t.Fatal("orphaned resolution message", err)
	}
}
