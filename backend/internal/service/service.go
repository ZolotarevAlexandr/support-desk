package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"github.com/uptrace/bun"
	"golang.org/x/crypto/bcrypt"
	"net/mail"
	"sre_tbank/internal/metrics"
	"sre_tbank/internal/store"
	"strings"
	"time"
	"unicode/utf8"
)

type Error struct {
	Status        int
	Code, Message string
	Fields        map[string]string
}

func (e *Error) Error() string { return e.Message }

func Bad(message string) error { return &Error{400, "validation", message, nil} }

func Forbidden() error {
	return &Error{403, "forbidden", "This action requires operator permissions", nil}
}

func conflict(message string) error { return &Error{409, "conflict", message, nil} }

func Normalize(err error) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return &Error{404, "not_found", "Resource not found", nil}
	case errors.Is(err, store.ErrConflict):
		return conflict("Email or category name already exists")
	case errors.Is(err, store.ErrReference):
		return conflict("Related record is missing or still referenced; recategorize tickets before deleting a category")
	}

	return err
}

type Service struct {
	Store           *store.Store
	Metrics         *metrics.Metrics
	InviteCode      string
	SessionDuration time.Duration
}

type RegisterInput struct {
	Name                 string `json:"name"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
	Role                 string `json:"role"`
	InviteCode           string `json:"invite_code"`
}

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func text(value string, max int, required bool) (string, error) {
	value = strings.TrimSpace(value)
	n := utf8.RuneCountInString(value)
	if n > max || (required && n == 0) {
		return "", Bad("Text is required and must respect the field length limit")
	}

	return value, nil
}

func email(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	a, err := mail.ParseAddress(value)
	if err != nil || a.Address != value || len(value) > 254 {
		return "", Bad("Enter a valid email address")
	}

	return value, nil
}

func password(value string) error {
	if utf8.RuneCountInString(value) < 12 || len(value) > 72 {
		return Bad("Password must contain at least 12 characters and at most 72 bytes")
	}

	return nil
}

func token() (string, error) {
	v := make([]byte, 32)
	_, err := rand.Read(v)
	return hex.EncodeToString(v), err
}

func HashToken(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }

func (s *Service) session() (string, *store.Session, error) {
	v, err := token()
	return v, &store.Session{TokenHash: HashToken(v), ExpiresAt: time.Now().UTC().Add(s.SessionDuration)}, err
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (store.User, string, error) {
	var u store.User
	var err error
	if u.Name, err = text(in.Name, 100, true); err != nil {
		return u, "", err
	}

	if u.Email, err = email(in.Email); err != nil {
		return u, "", err
	}

	if err = password(in.Password); err != nil {
		return u, "", err
	}

	if in.Password != in.PasswordConfirmation {
		return u, "", Bad("Passwords do not match")
	}

	if in.Role != "user" && in.Role != "operator" {
		return u, "", Bad("Choose User or Operator")
	}

	if in.Role == "operator" && (s.InviteCode == "" || subtle.ConstantTimeCompare([]byte(in.InviteCode), []byte(s.InviteCode)) != 1) {
		return u, "", &Error{403, "invitation_required", "A valid operator invitation is required", nil}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), 12)
	if err != nil {
		return u, "", err
	}

	u.PasswordHash = string(hash)
	u.Role = in.Role

	value, session, err := s.session()
	if err != nil {
		return u, "", err
	}

	err = s.Store.Register(ctx, &u, session)
	return u, value, Normalize(err)
}

// A fixed valid hash makes unknown-account login perform the same bcrypt work.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), 12)

func (s *Service) Login(ctx context.Context, in LoginInput) (store.User, string, error) {
	u, err := s.Store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return u, "", err
	}

	hash := []byte(u.PasswordHash)
	if errors.Is(err, store.ErrNotFound) {
		hash = dummyHash
	}

	if bcrypt.CompareHashAndPassword(hash, []byte(in.Password)) != nil || err != nil {
		return store.User{}, "", &Error{401, "invalid_credentials", "Email or password is incorrect", nil}
	}

	value, v, err := s.session()
	if err != nil {
		return u, "", err
	}

	v.UserID = u.ID
	err = s.Store.CreateSession(ctx, v)
	if err == nil {
		s.Metrics.Login(u.Role)
	}
	return u, value, Normalize(err)
}

func (s *Service) Authenticate(ctx context.Context, value string) (store.User, error) {
	if len(value) != 64 {
		return store.User{}, &Error{401, "unauthorized", "Please sign in", nil}
	}

	u, err := s.Store.SessionUser(ctx, HashToken(value))
	if errors.Is(err, store.ErrNotFound) {
		return u, &Error{401, "unauthorized", "Your session has expired; please sign in", nil}
	}

	return u, err
}

func (s *Service) Logout(ctx context.Context, value string) error {
	return s.Store.Revoke(ctx, HashToken(value))
}

func Operator(u store.User) error {
	if u.Role != "operator" {
		return Forbidden()
	}

	return nil
}

func (s *Service) Categories(ctx context.Context) ([]store.Category, error) {
	return s.Store.Categories(ctx)
}

func (s *Service) Category(ctx context.Context, id int64) (store.Category, error) {
	v, err := s.Store.Category(ctx, id)
	return v, Normalize(err)
}

type CategoryInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (s *Service) SaveCategory(ctx context.Context, u store.User, id int64, in CategoryInput) (store.Category, error) {
	v := store.Category{}
	if err := Operator(u); err != nil {
		return v, err
	}

	var err error
	if id != 0 {
		v, err = s.Store.Category(ctx, id)
		if err != nil {
			return v, Normalize(err)
		}
	} else if in.Name == nil {
		return v, Bad("Name is required")
	}

	if in.Name != nil {
		v.Name, err = text(*in.Name, 100, true)
		if err != nil {
			return v, err
		}
	}

	if in.Description != nil {
		v.Description, err = text(*in.Description, 1000, false)
		if err != nil {
			return v, err
		}
	}

	err = s.Store.SaveCategory(ctx, &v)
	return v, Normalize(err)
}

func (s *Service) DeleteCategory(ctx context.Context, u store.User, id int64) error {
	if err := Operator(u); err != nil {
		return err
	}

	return Normalize(s.Store.DeleteCategory(ctx, id))
}

func (s *Service) Operators(ctx context.Context, u store.User) ([]store.User, error) {
	if err := Operator(u); err != nil {
		return nil, err
	}

	return s.Store.Operators(ctx)
}

func valid(v string, choices ...string) bool {
	for _, c := range choices {
		if v == c {
			return true
		}
	}

	return false
}

func ValidateFilter(f store.Filter) error {
	if f.Status != "" && !valid(f.Status, "open", "in_progress", "resolved", "closed") {
		return Bad("Invalid status")
	}

	if f.Priority != "" && !valid(f.Priority, "low", "medium", "high", "urgent") {
		return Bad("Invalid priority")
	}

	if f.Assignment != "" && !valid(f.Assignment, "me", "unassigned") {
		return Bad("Invalid assignment filter")
	}

	if len(f.Search) > 1000 {
		return Bad("Search is too long")
	}

	return nil
}

func (s *Service) Tickets(ctx context.Context, u store.User, f store.Filter) (store.Page[store.Ticket], error) {
	if err := ValidateFilter(f); err != nil {
		return store.Page[store.Ticket]{}, err
	}

	return s.Store.Tickets(ctx, u, f)
}

func (s *Service) Ticket(ctx context.Context, u store.User, id int64) (store.Ticket, error) {
	v, err := s.Store.Ticket(ctx, u, id)
	return v, Normalize(err)
}

type TicketInput struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	CategoryID  *int64  `json:"category_id"`
	Priority    *string `json:"priority"`
}

func applyTicket(v *store.Ticket, in TicketInput) error {
	var err error
	if in.Title != nil {
		v.Title, err = text(*in.Title, 200, true)
		if err != nil {
			return err
		}
	}

	if in.Description != nil {
		v.Description, err = text(*in.Description, 10000, true)
		if err != nil {
			return err
		}
	}

	if in.CategoryID != nil {
		if *in.CategoryID < 1 {
			return Bad("Choose a category")
		}

		v.CategoryID = *in.CategoryID
	}

	if in.Priority != nil {
		if !valid(*in.Priority, "low", "medium", "high", "urgent") {
			return Bad("Invalid priority")
		}

		v.Priority = *in.Priority
	}

	return nil
}

func (s *Service) CreateTicket(ctx context.Context, u store.User, in TicketInput) (store.Ticket, error) {
	v := store.Ticket{RequesterID: u.ID, RequesterName: u.Name, Priority: "medium", Status: "open"}
	if in.Title == nil || in.Description == nil || in.CategoryID == nil {
		return v, Bad("Title, description and category are required")
	}

	if err := applyTicket(&v, in); err != nil {
		return v, err
	}

	err := s.Store.CreateTicket(ctx, &v)
	if err == nil {
		s.Metrics.TicketOpened(v.Priority)
	}
	return v, Normalize(err)
}

func canEditTicket(u store.User, v *store.Ticket) error {
	if u.Role != "operator" && v.Status != "open" {
		return conflict("Only open tickets can be edited or deleted by their requester")
	}

	return nil
}

func (s *Service) UpdateTicket(ctx context.Context, u store.User, id int64, in TicketInput) (store.Ticket, error) {
	var result store.Ticket
	err := s.Store.MutateTicket(ctx, u, id, func(ctx context.Context, tx bun.Tx, v *store.Ticket) error {
		if err := canEditTicket(u, v); err != nil {
			return err
		}

		if err := applyTicket(v, in); err != nil {
			return err
		}

		if err := store.SaveTicket(ctx, tx, v); err != nil {
			return err
		}

		result = *v
		return nil
	})

	return result, Normalize(err)
}

func (s *Service) DeleteTicket(ctx context.Context, u store.User, id int64) error {
	return Normalize(s.Store.MutateTicket(ctx, u, id, func(ctx context.Context, tx bun.Tx, v *store.Ticket) error {
		if err := canEditTicket(u, v); err != nil {
			return err
		}

		return store.DeleteTicket(ctx, tx, id)
	}))
}

func (s *Service) Comments(ctx context.Context, u store.User, id int64, page, size int) (store.Page[store.Comment], error) {
	v, err := s.Store.Comments(ctx, u, id, page, size)
	return v, Normalize(err)
}

func (s *Service) MutateComment(ctx context.Context, u store.User, ticket, id int64, body string, remove bool) (store.Comment, error) {
	var result store.Comment
	var err error
	if !remove {
		body, err = text(body, 5000, true)
		if err != nil {
			return result, err
		}
	}

	err = s.Store.MutateTicket(ctx, u, ticket, func(ctx context.Context, tx bun.Tx, v *store.Ticket) error {
		if v.Status == "closed" {
			return conflict("Closed ticket conversations are read-only")
		}

		if id == 0 {
			result = store.Comment{TicketID: ticket, AuthorID: u.ID, Body: body}
			return store.InsertComment(ctx, tx, &result)
		}

		result, err = store.GetComment(ctx, tx, ticket, id)
		if err != nil {
			return err
		}

		if result.AuthorID != u.ID && !(remove && u.Role == "operator") {
			return &Error{403, "forbidden", "You can only edit your own comments", nil}
		}

		if remove {
			return store.DeleteComment(ctx, tx, id)
		}

		result.Body = body
		return store.SaveComment(ctx, tx, &result)
	})

	result.AuthorName = u.Name
	result.AuthorRole = u.Role
	if err == nil && id == 0 && !remove {
		s.Metrics.CommentAdded(u.Role)
	}
	return result, Normalize(err)
}
