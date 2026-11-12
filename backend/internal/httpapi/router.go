package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"sre_tbank/internal/config"
	"sre_tbank/internal/service"
	"sre_tbank/internal/staticui"
	"sre_tbank/internal/store"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type API struct {
	Service      *service.Service
	Config       config.Config
	Logger       *slog.Logger
	ShuttingDown atomic.Bool
}

func (a *API) Handler() http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(a.middleware(), a.httpMetrics(), a.recovery())

	// Metrics are served exclusively by the internal monitoring listener.
	r.Any("/metrics", func(c *gin.Context) { c.Status(http.StatusNotFound) })

	// Intentional failure for the logging lab; middleware records it at ERROR.
	r.GET("/debug/error", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Intentional logging lab error"})
	})

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) {
		if a.ShuttingDown.Load() || a.Service.Store.DB.PingContext(c.Request.Context()) != nil {
			a.fail(c, &service.Error{Status: 503, Code: "unavailable", Message: "Database unavailable or application shutting down"})
			return
		}

		c.JSON(200, gin.H{"status": "ready"})
	})

	g := r.Group("/api/v1")
	g.POST("/auth/register", func(c *gin.Context) {
		var in service.RegisterInput
		if !a.decode(c, &in) {
			return
		}

		u, t, err := a.Service.Register(c.Request.Context(), in)
		if err != nil {
			a.fail(c, err)
			return
		}

		a.cookie(c, t)
		c.JSON(201, u)
	})

	g.POST("/auth/login", func(c *gin.Context) {
		var in service.LoginInput
		if !a.decode(c, &in) {
			return
		}

		u, t, err := a.Service.Login(c.Request.Context(), in)
		if err != nil {
			a.fail(c, err)
			return
		}

		a.cookie(c, t)
		c.JSON(200, u)
	})

	g.POST("/auth/logout", func(c *gin.Context) {
		v, _ := c.Cookie("support_session")
		if err := a.Service.Logout(c.Request.Context(), v); err != nil {
			a.fail(c, err)
			return
		}

		a.cookie(c, "")
		c.Status(204)
	})

	g.Use(a.auth())
	g.GET("/auth/me", func(c *gin.Context) { c.JSON(200, user(c)) })
	g.GET("/operators", func(c *gin.Context) {
		v, err := a.Service.Operators(c.Request.Context(), user(c))
		a.result(c, 200, v, err)
	})

	g.GET("/categories", func(c *gin.Context) { v, err := a.Service.Categories(c.Request.Context()); a.result(c, 200, v, err) })
	g.GET("/categories/:id", func(c *gin.Context) {
		id, ok := a.id(c, "id")
		if !ok {
			return
		}

		v, err := a.Service.Category(c.Request.Context(), id)
		a.result(c, 200, v, err)
	})

	g.POST("/categories", a.saveCategory(false))
	g.PATCH("/categories/:id", a.saveCategory(true))
	g.DELETE("/categories/:id", func(c *gin.Context) {
		id, ok := a.id(c, "id")
		if !ok {
			return
		}

		a.result(c, 204, nil, a.Service.DeleteCategory(c.Request.Context(), user(c), id))
	})

	g.GET("/tickets", func(c *gin.Context) {
		page, size, ok := a.pagination(c)
		if !ok {
			return
		}

		f := store.Filter{Page: page, PageSize: size, Search: c.Query("search"), Status: c.Query("status"), Priority: c.Query("priority"), Assignment: c.Query("assignment")}
		if s := c.Query("category_id"); s != "" {
			id, err := strconv.ParseInt(s, 10, 64)
			if err != nil || id < 1 {
				a.fail(c, service.Bad("Invalid category"))
				return
			}

			f.CategoryID = id
		}

		v, err := a.Service.Tickets(c.Request.Context(), user(c), f)
		a.result(c, 200, v, err)
	})

	g.POST("/tickets", a.saveTicket(false))
	g.PATCH("/tickets/:id", a.saveTicket(true))
	g.POST("/tickets/:id/actions", func(c *gin.Context) {
		id, ok := a.id(c, "id")
		if !ok {
			return
		}

		var raw json.RawMessage
		if !a.decode(c, &raw) {
			return
		}

		var in service.TicketActionInput
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			a.fail(c, service.Bad("Invalid action fields"))
			return
		}

		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil || keys == nil {
			a.fail(c, service.Bad("Expected an action object"))
			return
		}

		_, in.AssigneeSet = keys["assignee_id"]
		if value, exists := keys["message"]; exists && string(value) == "null" {
			a.fail(c, service.Bad("Message cannot be null"))
			return
		}

		ticket, err := a.Service.TicketAction(c.Request.Context(), user(c), id, in)
		a.result(c, 200, ticket, err)
	})

	g.GET("/tickets/:id", func(c *gin.Context) {
		id, ok := a.id(c, "id")
		if !ok {
			return
		}

		v, err := a.Service.Ticket(c.Request.Context(), user(c), id)
		a.result(c, 200, v, err)
	})

	g.DELETE("/tickets/:id", func(c *gin.Context) {
		id, ok := a.id(c, "id")
		if !ok {
			return
		}

		a.result(c, 204, nil, a.Service.DeleteTicket(c.Request.Context(), user(c), id))
	})

	g.GET("/tickets/:id/comments", func(c *gin.Context) {
		id, ok := a.id(c, "id")
		if !ok {
			return
		}

		page, size, ok := a.pagination(c)
		if !ok {
			return
		}

		v, err := a.Service.Comments(c.Request.Context(), user(c), id, page, size)
		a.result(c, 200, v, err)
	})

	g.POST("/tickets/:id/comments", a.comment("create"))
	g.PATCH("/tickets/:id/comments/:comment_id", a.comment("edit"))
	g.DELETE("/tickets/:id/comments/:comment_id", a.comment("delete"))
	assets := staticui.Assets()
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if p == "/api" || strings.HasPrefix(p, "/api/") {
			a.fail(c, &service.Error{Status: 404, Code: "not_found", Message: "API endpoint not found"})
			return
		}

		if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			c.Status(404)
			return
		}

		name := strings.TrimPrefix(path.Clean(p), "/")
		if name == "" {
			name = "index.html"
		}

		if data, err := fs.ReadFile(assets, name); err == nil {
			http.ServeContent(c.Writer, c.Request, name, time.Time{}, strings.NewReader(string(data)))
			return
		}

		if strings.HasPrefix(p, "/assets/") || path.Ext(p) != "" {
			c.Status(404)
			return
		}

		data, _ := fs.ReadFile(assets, "index.html")
		c.Data(200, "text/html; charset=utf-8", data)
	})

	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		_ = json.NewEncoder(w).Encode(gin.H{"error": gin.H{"code": "cross_origin", "message": "Cross-origin mutation rejected"}})
	}))

	return cop.Handler(r)
}

// Gin's default recovery dumps request headers. Keep recovery logs sanitized.
func (a *API) recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				c.Abort()
				a.Logger.Error("panic recovered", "request_id", c.Writer.Header().Get("X-Request-ID"))
				if !c.Writer.Written() {
					a.fail(c, errors.New("panic recovered"))
				}
			}
		}()

		c.Next()
	}
}

func (a *API) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		requestID := hex.EncodeToString(b)
		c.Header("X-Request-ID", requestID)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")

		ctx, cancel := context.WithTimeout(c.Request.Context(), a.Config.QueryTimeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024)

		c.Next()

		level := slog.LevelInfo
		if c.Writer.Status() >= 500 {
			level = slog.LevelError
		} else if c.Writer.Status() >= 400 {
			level = slog.LevelWarn
		}
		a.Logger.Log(c.Request.Context(), level, "request", "request_id", requestID, "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
	}
}

func (a *API) auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		v, _ := c.Cookie("support_session")
		u, err := a.Service.Authenticate(c.Request.Context(), v)
		if err != nil {
			a.fail(c, err)
			c.Abort()
			return
		}

		c.Set("user", u)

		c.Next()
	}
}

func user(c *gin.Context) store.User { return c.MustGet("user").(store.User) }

func (a *API) cookie(c *gin.Context, v string) {
	maxAge := int(a.Config.SessionDuration.Seconds())
	expires := time.Now().Add(a.Config.SessionDuration)
	if v == "" {
		maxAge = -1
		expires = time.Unix(1, 0)
	}

	http.SetCookie(c.Writer, &http.Cookie{Name: "support_session", Value: v, Path: "/", MaxAge: maxAge, Expires: expires, Secure: a.Config.SecureCookie, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (a *API) fail(c *gin.Context, err error) {
	err = service.Normalize(err)

	var e *service.Error
	if !errors.As(err, &e) {
		// Correlate diagnostics without exposing driver errors or request values.
		a.Logger.Error("request failed", "request_id", c.Writer.Header().Get("X-Request-ID"), "error.type", fmt.Sprintf("%T", err))
		if errors.Is(err, context.DeadlineExceeded) {
			e = &service.Error{Status: 503, Code: "unavailable", Message: "Operation timed out; please try again"}
		} else {
			e = &service.Error{Status: 500, Code: "internal", Message: "An unexpected error occurred"}
		}
	}

	c.JSON(e.Status, gin.H{"error": gin.H{"code": e.Code, "message": e.Message, "fields": e.Fields}})
}

func (a *API) result(c *gin.Context, status int, v any, err error) {
	if err != nil {
		a.fail(c, err)
		return
	}

	if status == 204 {
		c.Status(status)
		return
	}

	c.JSON(status, v)
}

func (a *API) decode(c *gin.Context, v any) bool {
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		a.fail(c, service.Bad("Invalid JSON or unsupported field"))
		return false
	}

	if err := d.Decode(new(any)); err != io.EOF {
		a.fail(c, service.Bad("Expected one JSON object"))
		return false
	}

	return true
}

func (a *API) id(c *gin.Context, key string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil || id < 1 {
		a.fail(c, service.Bad("Invalid resource ID"))
		return 0, false
	}

	return id, true
}

func (a *API) pagination(c *gin.Context) (int, int, bool) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, e := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	if err != nil || e != nil || page < 1 || page > 1000000 || size < 1 || size > 100 {
		a.fail(c, service.Bad("Page must be positive and page_size between 1 and 100"))
		return 0, 0, false
	}

	return page, size, true
}

func (a *API) saveCategory(update bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var id int64
		if update {
			var ok bool
			id, ok = a.id(c, "id")
			if !ok {
				return
			}
		}

		var in service.CategoryInput
		if !a.decode(c, &in) {
			return
		}

		v, err := a.Service.SaveCategory(c.Request.Context(), user(c), id, in)
		status := 201
		if update {
			status = 200
		}

		a.result(c, status, v, err)
	}
}

func (a *API) saveTicket(update bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var id int64
		if update {
			var ok bool
			id, ok = a.id(c, "id")
			if !ok {
				return
			}
		}

		var raw json.RawMessage
		if !a.decode(c, &raw) {
			return
		}

		var in service.TicketInput
		d := json.NewDecoder(strings.NewReader(string(raw)))
		d.DisallowUnknownFields()
		if err := d.Decode(&in); err != nil {
			a.fail(c, service.Bad("Invalid ticket fields"))
			return
		}

		var keys map[string]json.RawMessage
		if err := json.Unmarshal(raw, &keys); err != nil || keys == nil {
			a.fail(c, service.Bad("Expected a JSON object"))
			return
		}

		for _, v := range keys {
			if string(v) == "null" {
				a.fail(c, service.Bad("Ticket fields cannot be null"))
				return
			}
		}

		if update {
			v, err := a.Service.UpdateTicket(c.Request.Context(), user(c), id, in)
			a.result(c, 200, v, err)
		} else {
			v, err := a.Service.CreateTicket(c.Request.Context(), user(c), in)
			a.result(c, 201, v, err)
		}
	}
}

func (a *API) comment(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ticket, ok := a.id(c, "id")
		if !ok {
			return
		}

		var id int64
		if action != "create" {
			id, ok = a.id(c, "comment_id")
			if !ok {
				return
			}
		}

		var in struct {
			Body string `json:"body"`
		}

		if action != "delete" && !a.decode(c, &in) {
			return
		}

		v, err := a.Service.MutateComment(c.Request.Context(), user(c), ticket, id, in.Body, action == "delete")
		status := 200
		if action == "create" {
			status = 201
		}

		if action == "delete" {
			status = 204
		}

		a.result(c, status, v, err)
	}
}
