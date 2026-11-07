package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"sre_tbank/internal/store"
)

func TestLabFaultsAndCancellation(t *testing.T) {
	if err := (FaultInjector{}).Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	var failure *Error
	if err := (FaultInjector{Rate: 1}).Apply(context.Background()); !errors.As(err, &failure) || failure.Status != 503 {
		t.Fatalf("expected injected 503, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := (FaultInjector{Delay: time.Hour}).Apply(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("canceled delay did not stop promptly")
	}
}

func TestEligibleBusinessAttempts(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, 1},
		{"validation", Bad("invalid"), 0},
		{"forbidden", Forbidden(), 0},
		{"missing", store.ErrNotFound, 0},
		{"conflict", store.ErrConflict, 0},
		{"reference", store.ErrReference, 0},
		{"internal", errors.New("database down"), 1},
		{"deadline", context.DeadlineExceeded, 1},
		{"injected", &Error{Status: 503}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := 0
			businessAttempt(tc.err, func() { got++ })
			if got != tc.want {
				t.Fatalf("count=%d want=%d", got, tc.want)
			}
		})
	}
}
