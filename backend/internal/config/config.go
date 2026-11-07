package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL, Port, MetricsPort, LogLevel, InviteCode string
	PoolSize                                             int
	QueryTimeout, ShutdownTimeout, SessionDuration       time.Duration
	LabFaultRate                                         float64
	LabFaultDelay                                        time.Duration
	SecureCookie                                         bool
}

func Load() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), Port: env("PORT", "8080"), MetricsPort: env("METRICS_PORT", "9090"), LogLevel: env("LOG_LEVEL", "info"), InviteCode: os.Getenv("OPERATOR_INVITE_CODE")}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}

	port, err := strconv.Atoi(c.Port)
	if err != nil || port < 1 || port > 65535 {
		return c, fmt.Errorf("invalid PORT")
	}
	metricsPort, err := strconv.Atoi(c.MetricsPort)
	if err != nil || metricsPort < 1 || metricsPort > 65535 {
		return c, fmt.Errorf("invalid METRICS_PORT")
	}
	if metricsPort == port {
		return c, fmt.Errorf("METRICS_PORT must differ from PORT")
	}

	c.PoolSize, err = strconv.Atoi(env("DB_POOL_SIZE", "20"))
	if err != nil || c.PoolSize < 1 {
		return c, fmt.Errorf("invalid DB_POOL_SIZE")
	}

	for key, dst := range map[string]*time.Duration{"QUERY_TIMEOUT": &c.QueryTimeout, "SHUTDOWN_TIMEOUT": &c.ShutdownTimeout, "SESSION_DURATION": &c.SessionDuration} {
		def := map[string]string{"QUERY_TIMEOUT": "5s", "SHUTDOWN_TIMEOUT": "15s", "SESSION_DURATION": "24h"}[key]
		*dst, err = time.ParseDuration(env(key, def))
		if err != nil || *dst <= 0 {
			return c, fmt.Errorf("invalid %s", key)
		}
	}

	c.SecureCookie, err = strconv.ParseBool(env("SECURE_COOKIE", "false"))
	if err != nil {
		return c, fmt.Errorf("invalid SECURE_COOKIE")
	}
	c.LabFaultRate, err = strconv.ParseFloat(env("LAB_FAULT_RATE", "0"), 64)
	if err != nil || math.IsNaN(c.LabFaultRate) || math.IsInf(c.LabFaultRate, 0) || c.LabFaultRate < 0 || c.LabFaultRate > 1 {
		return c, fmt.Errorf("invalid LAB_FAULT_RATE")
	}
	c.LabFaultDelay, err = time.ParseDuration(env("LAB_FAULT_DELAY", "0s"))
	if err != nil || c.LabFaultDelay < 0 {
		return c, fmt.Errorf("invalid LAB_FAULT_DELAY")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return c, fmt.Errorf("invalid LOG_LEVEL")
	}

	return c, nil
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}

	return d
}
