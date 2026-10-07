package httpapi

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoveryDoesNotLogHeadersOrPanicPayload(t *testing.T) {
	var logs bytes.Buffer
	a := &API{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	r := gin.New()
	r.Use(a.recovery())
	r.GET("/panic", func(c *gin.Context) { panic("private-invite-secret") })

	request := httptest.NewRequest("GET", "/panic", nil)
	request.Header.Set("Cookie", "support_session=private-cookie-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request)

	if w.Code != 500 {
		t.Fatalf("status %d", w.Code)
	}

	for _, value := range []string{"private-invite-secret", "private-cookie-secret"} {
		if strings.Contains(logs.String(), value) || strings.Contains(w.Body.String(), value) {
			t.Fatalf("recovery leaked secret")
		}
	}
}
