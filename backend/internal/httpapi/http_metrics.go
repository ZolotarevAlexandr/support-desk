package httpapi

import (
	"strings"

	"github.com/gin-gonic/gin"
	"sre_tbank/internal/metrics"
)

func (a *API) httpMetrics() gin.HandlerFunc {
	var observations *metrics.Metrics
	if a.Service != nil {
		observations = a.Service.Metrics
	}
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if p != "/api" && !strings.HasPrefix(p, "/api/") && p != "/debug/error" {
			c.Next()
			return
		}
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		complete := observations.StartHTTPRequest(c.Request.Method, route)
		defer func() { complete(c.Writer.Status()) }()
		c.Next()
	}
}
