package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pressly/goose/v3"
	"github.com/uptrace/bun"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sre_tbank/internal/config"
	"sre_tbank/internal/service"
	"sre_tbank/internal/store"
	"sre_tbank/migrations"
	"strings"
	"sync"
	"testing"
	"time"
)

type harness struct {
	t       *testing.T
	a       *API
	handler http.Handler
}

func setup(t *testing.T) *harness {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests (use a disposable database)")
	}

	s := store.Open(dsn, 10)
	t.Cleanup(func() { _ = s.DB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.DB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	goose.SetBaseFS(migrations.Files)
	_ = goose.SetDialect("postgres")
	if err := goose.UpContext(ctx, s.DB.DB, "."); err != nil {
		t.Fatal(err)
	}

	if err := goose.UpContext(ctx, s.DB.DB, "."); err != nil {
		t.Fatal(err)
	}

	if _, err := s.DB.ExecContext(ctx, "TRUNCATE comments,tickets,categories,sessions,users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}

	if err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	if err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	c := config.Config{QueryTimeout: 10 * time.Second, SessionDuration: time.Hour, InviteCode: "private-invite"}
	a := &API{Service: &service.Service{Store: s, SessionDuration: c.SessionDuration, InviteCode: c.InviteCode}, Config: c, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	return &harness{t, a, a.Handler()}
}

func (h *harness) request(method, path, token string, body any, want int) *httptest.ResponseRecorder {
	h.t.Helper()

	var b []byte
	if body != nil {
		var err error
		b, err = json.Marshal(body)
		if err != nil {
			h.t.Fatal(err)
		}
	}

	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: "support_session", Value: token})
	}

	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)

	if w.Code != want {
		h.t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
	}

	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()

	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}

	return v
}

func (h *harness) register(name, role string) (store.User, string) {
	h.t.Helper()
	w := h.request("POST", "/api/v1/auth/register", "", map[string]any{"name": name, "email": name + "@example.com", "password": "strong-password-123", "password_confirmation": "strong-password-123", "role": role, "invite_code": "private-invite"}, 201)
	return decode[store.User](h.t, w), w.Result().Cookies()[0].Value
}

func (h *harness) ticket(token, title string) store.Ticket {
	return decode[store.Ticket](h.t, h.request("POST", "/api/v1/tickets", token, map[string]any{"title": title, "description": "Detailed issue", "category_id": 1}, 201))
}

func TestWorkflowAndPermissions(t *testing.T) {
	h := setup(t)
	u, ut := h.register("alice", "user")
	_, other := h.register("bob", "user")
	op, ot := h.register("operator", "operator")
	ctx := context.Background()

	cats := decode[[]store.Category](t, h.request("GET", "/api/v1/categories", ut, nil, 200))
	if len(cats) != 4 {
		t.Fatalf("seed should be repeatable: %d", len(cats))
	}

	h.request("GET", "/api/v1/operators", ut, nil, 403)
	operators := decode[[]store.User](t, h.request("GET", "/api/v1/operators", ot, nil, 200))
	if len(operators) != 1 {
		t.Fatal("missing operator")
	}

	h.request("POST", "/api/v1/categories", ut, map[string]any{"name": "Forbidden"}, 403)
	c := decode[store.Category](t, h.request("POST", "/api/v1/categories", ot, map[string]any{"name": "Other", "description": "Description"}, 201))
	h.request("PATCH", fmt.Sprintf("/api/v1/categories/%d", c.ID), ot, map[string]any{"name": "Other updated"}, 200)
	h.request("POST", "/api/v1/categories", ot, map[string]any{"name": "OTHER UPDATED"}, 409)
	h.request("DELETE", fmt.Sprintf("/api/v1/categories/%d", c.ID), ot, nil, 204)

	a := h.ticket(ut, "Printer broken")
	_ = h.ticket(other, "Private printer broken")
	detail := fmt.Sprintf("/api/v1/tickets/%d", a.ID)
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		var body any
		if method == "PATCH" {
			body = map[string]any{"title": "Intrusion"}
		}

		h.request(method, detail, other, body, 404)
	}

	for _, suffix := range []string{"", "?search=Printer", "?search=Printer&status=open&priority=medium&category_id=1&assignment=unassigned"} {
		p := decode[store.Page[store.Ticket]](t, h.request("GET", "/api/v1/tickets"+suffix, ut, nil, 200))
		if p.Total != 1 || len(p.Items) != 1 || p.Items[0].RequesterID != u.ID {
			t.Fatalf("isolation/count mismatch: %+v", p)
		}
	}

	p := decode[store.Page[store.Ticket]](t, h.request("GET", "/api/v1/tickets?status=resolved", ut, nil, 200))
	if p.Total != 0 || len(p.Items) != 0 {
		t.Fatal("filter mismatch")
	}

	h.request("GET", "/api/v1/tickets?page_size=101", ut, nil, 400)
	h.request("GET", "/api/v1/tickets?status=bad", ut, nil, 400)
	h.request("PATCH", detail, ut, map[string]any{"requester_id": op.ID}, 400)
	h.request("PATCH", detail, ut, map[string]any{"status": "resolved"}, 400)
	h.request("PATCH", detail, ot, map[string]any{"assignee_id": u.ID}, 400)
	h.request("PATCH", detail, ut, map[string]any{"title": "Printer still broken"}, 200)
	h.action(ot, a.ID, map[string]any{"action": "claim"}, 200)
	h.request("PATCH", detail, ut, map[string]any{"title": "Cannot edit"}, 409)
	h.request("DELETE", detail, ut, nil, 409)
	assigned := decode[store.Page[store.Ticket]](t, h.request("GET", "/api/v1/tickets?assignment=me", ot, nil, 200))
	if assigned.Total != 1 {
		t.Fatal("assignment filter")
	}

	base := detail + "/comments"
	h.request("GET", base, other, nil, 404)
	h.request("POST", base, other, map[string]any{"body": "intrusion"}, 404)
	own := decode[store.Comment](t, h.request("POST", base, ut, map[string]any{"body": "Please help"}, 201))
	reply := decode[store.Comment](t, h.request("POST", base, ot, map[string]any{"body": "Installing the driver"}, 201))
	ownPath := fmt.Sprintf("%s/%d", base, own.ID)
	replyPath := fmt.Sprintf("%s/%d", base, reply.ID)
	h.request("PATCH", ownPath, ot, map[string]any{"body": "not mine"}, 403)
	h.request("PATCH", replyPath, ut, map[string]any{"body": "not mine"}, 403)
	h.request("DELETE", replyPath, ut, nil, 403)
	h.request("PATCH", ownPath, ut, map[string]any{"body": "Updated question"}, 200)
	h.request("PATCH", replyPath, ot, map[string]any{"body": "Driver installed"}, 200)
	messages := decode[store.Page[store.Comment]](t, h.request("GET", base+"?page_size=1", ut, nil, 200))
	if messages.Total != 2 || len(messages.Items) != 1 || messages.Items[0].AuthorName != u.Name {
		t.Fatal("comment pagination/author")
	}

	h.request("DELETE", "/api/v1/categories/1", ot, nil, 409)
	h.action(ot, a.ID, map[string]any{"action": "resolve", "message": "Driver installed; please confirm"}, 200)
	h.request("POST", base, ut, map[string]any{"body": "Thanks"}, 201)
	h.action(ut, a.ID, map[string]any{"action": "confirm"}, 200)
	h.request("POST", base, ut, map[string]any{"body": "Too late"}, 409)
	h.request("PATCH", replyPath, ot, map[string]any{"body": "Too late"}, 409)
	h.request("DELETE", ownPath, ot, nil, 409)
	h.action(ot, a.ID, map[string]any{"action": "reopen", "message": "Follow-up needed"}, 200)
	h.action(ot, a.ID, map[string]any{"action": "reassign", "assignee_id": nil}, 200)
	h.request("DELETE", ownPath, ot, nil, 204)
	h.request("DELETE", replyPath, ot, nil, 204)
	h.request("DELETE", detail, ut, nil, 204)

	var count int
	if err := h.a.Service.Store.DB.NewSelect().Model((*store.Comment)(nil)).Where("ticket_id=?", a.ID).ColumnExpr("count(*)").Scan(ctx, &count); err != nil || count != 0 {
		t.Fatal("comment cascade", err, count)
	}

	h.request("GET", detail, ot, nil, 404)

	// A second handler uses the same persisted session, without sticky routing.
	second := *h
	second.a = &API{Service: h.a.Service, Config: h.a.Config, Logger: h.a.Logger}
	second.handler = second.a.Handler()
	second.request("GET", "/api/v1/auth/me", ut, nil, 200)
	h.request("POST", "/api/v1/auth/logout", ut, nil, 204)
	second.request("GET", "/api/v1/auth/me", ut, nil, 401)

	login := h.request("POST", "/api/v1/auth/login", "", map[string]any{"email": "ALICE@example.com", "password": "strong-password-123"}, 200)
	token := login.Result().Cookies()[0].Value
	if _, err := h.a.Service.Store.DB.ExecContext(ctx, "UPDATE sessions SET expires_at=now()-interval '1 second' WHERE token_hash=?", service.HashToken(token)); err != nil {
		t.Fatal(err)
	}

	h.request("GET", "/api/v1/auth/me", token, nil, 401)
	if err := h.a.Service.Store.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}

	h.request("GET", "/healthz", "", nil, 200)
	h.request("GET", "/readyz", "", nil, 200)
	h.a.ShuttingDown.Store(true)
	h.request("GET", "/readyz", "", nil, 503)
	h.request("GET", "/healthz", "", nil, 200)
	h.request("GET", "/api/v1/unknown", "", nil, 404)
	h.request("GET", "/assets/missing.js", "", nil, 404)
	h.request("GET", "/tickets/1", "", nil, 200)
}

func TestRegistrationAndCSRF(t *testing.T) {
	h := setup(t)
	body := map[string]any{"name": "Alice", "email": "alice@example.com", "password": "strong-password-123", "password_confirmation": "strong-password-123", "role": "operator"}
	h.request("POST", "/api/v1/auth/register", "", body, 403)
	body["invite_code"] = "incorrect"
	h.request("POST", "/api/v1/auth/register", "", body, 403)
	body["invite_code"] = "private-invite"
	h.a.Service.InviteCode = ""
	h.request("POST", "/api/v1/auth/register", "", body, 403)
	body["role"] = "user"
	w := h.request("POST", "/api/v1/auth/register", "", body, 201)
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("cookie flags")
	}

	if strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "private-invite") {
		t.Fatal("secret leak")
	}

	body["email"] = "ALICE@example.com"
	h.request("POST", "/api/v1/auth/register", "", body, 409)

	body["email"] = "other@example.com"
	body["password"] = "short"
	body["password_confirmation"] = "short"
	h.request("POST", "/api/v1/auth/register", "", body, 400)

	body["password"] = strings.Repeat("é", 37)
	body["password_confirmation"] = body["password"]
	h.request("POST", "/api/v1/auth/register", "", body, 400)

	body["password"] = "strong-password-123"
	body["password_confirmation"] = "wrong"
	h.request("POST", "/api/v1/auth/register", "", body, 400)

	h.request("POST", "/api/v1/auth/login", "", map[string]any{"email": "alice@example.com", "password": "wrong"}, 401)

	h.request("POST", "/api/v1/auth/login", "", map[string]any{"email": "missing@example.com", "password": "wrong"}, 401)
	for _, route := range []string{"/auth/login", "/auth/register", "/auth/logout", "/tickets"} {
		r := httptest.NewRequest("POST", "http://support.example/api/v1"+route, strings.NewReader("{}"))
		r.Header.Set("Origin", "https://attacker.example")
		w := httptest.NewRecorder()
		h.handler.ServeHTTP(w, r)
		if w.Code != 403 || !strings.Contains(w.Body.String(), "cross_origin") {
			t.Fatalf("CSRF %s: %d %s", route, w.Code, w.Body.String())
		}
	}
}

func TestConcurrentClosure(t *testing.T) {
	h := setup(t)
	u, ut := h.register("alice", "user")
	op, ot := h.register("operator", "operator")
	ctx := context.Background()
	for _, action := range []string{"ticket_edit", "comment_create", "comment_edit", "comment_delete"} {
		t.Run(action, func(t *testing.T) {
			ticket := h.ticket(ut, action)
			comment := decode[store.Comment](t, h.request("POST", fmt.Sprintf("/api/v1/tickets/%d/comments", ticket.ID), ut, map[string]any{"body": "Original"}, 201))

			locked := make(chan struct{})
			release := make(chan struct{})
			closeResult := make(chan error, 1)
			go func() {
				closeResult <- h.a.Service.Store.MutateTicket(ctx, op, ticket.ID, func(ctx context.Context, tx bun.Tx, v *store.Ticket) error {
					v.Status = "closed"
					if err := store.SaveTicket(ctx, tx, v); err != nil {
						return err
					}

					close(locked)
					<-release
					return nil
				})
			}()

			<-locked

			result := make(chan error, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				var err error
				switch action {
				case "ticket_edit":
					value := "Late edit"
					_, err = h.a.Service.UpdateTicket(ctx, u, ticket.ID, service.TicketInput{Title: &value})
				case "comment_create":
					_, err = h.a.Service.MutateComment(ctx, u, ticket.ID, 0, "Late message", false)
				case "comment_edit":
					_, err = h.a.Service.MutateComment(ctx, u, ticket.ID, comment.ID, "Late edit", false)
				case "comment_delete":
					_, err = h.a.Service.MutateComment(ctx, u, ticket.ID, comment.ID, "", true)
				}

				result <- err
			}()

			<-started
			select {
			case err := <-result:
				t.Fatalf("mutation passed the parent lock: %v", err)
			case <-time.After(60 * time.Millisecond):
			}

			close(release)
			if err := <-closeResult; err != nil {
				t.Fatal(err)
			}

			var e *service.Error
			if err := <-result; !errors.As(err, &e) || e.Status != 409 {
				t.Fatalf("expected closed restriction: %v", err)
			}

			// Reopen and run concurrent comments: all complete without loss.
			reason := "Resume work"
			if _, err := h.a.Service.TicketAction(ctx, op, ticket.ID, service.TicketActionInput{Action: "reopen", Message: &reason}); err != nil {
				t.Fatal(err)
			}

			var wg sync.WaitGroup
			failures := make(chan error, 5)
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := h.a.Service.MutateComment(ctx, u, ticket.ID, 0, "Concurrent message", false)
					failures <- err
				}()
			}

			wg.Wait()
			close(failures)
			for err := range failures {
				if err != nil {
					t.Fatal(err)
				}
			}

			_ = ot
		})
	}
}
