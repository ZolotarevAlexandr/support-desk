package service

import (
	"sre_tbank/internal/store"
	"strings"
	"testing"
)

func TestValidation(t *testing.T) {
	for _, p := range []string{"short", strings.Repeat("é", 37)} {
		if password(p) == nil {
			t.Fatal("accepted invalid password")
		}
	}

	if password(strings.Repeat("é", 36)) != nil {
		t.Fatal("72-byte password rejected")
	}

	if _, err := text("  ", 100, true); err == nil {
		t.Fatal("whitespace required text")
	}

	if _, err := text(strings.Repeat("界", 101), 100, true); err == nil {
		t.Fatal("unicode limit")
	}

	if _, err := email("Name <name@example.com>"); err == nil {
		t.Fatal("display names are not emails")
	}

	if ValidateFilter(store.Filter{Status: "invalid"}) == nil {
		t.Fatal("invalid status")
	}
}
