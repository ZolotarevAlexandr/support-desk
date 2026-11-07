package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/pressly/goose/v3"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sre_tbank/internal/config"
	"sre_tbank/internal/httpapi"
	"sre_tbank/internal/logging"
	"sre_tbank/internal/metrics"
	"sre_tbank/internal/service"
	"sre_tbank/internal/store"
	"sre_tbank/migrations"
	"syscall"
	"time"
)

var version = "development"

func main() {
	if err := run(); err != nil {
		// Database/driver errors can contain credentials or query values.
		slog.Error("application stopped", "error.type", fmt.Sprintf("%T", err))
		os.Exit(1)
	}
}

func run() error {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	environment := os.Getenv("DEPLOYMENT_ENVIRONMENT")
	if environment == "" {
		environment = "local"
	}
	level := new(slog.LevelVar)
	logger := logging.New(os.Stdout, level, version, environment, command)
	slog.SetDefault(logger)

	c, err := config.Load()
	if err != nil {
		// Config validation messages contain setting names, never their values.
		logger.Error("invalid configuration", "error.message", err.Error())
		return err
	}

	var configuredLevel slog.Level
	_ = configuredLevel.UnmarshalText([]byte(c.LogLevel))
	level.Set(configuredLevel)

	db := store.Open(c.DatabaseURL, c.PoolSize)
	defer db.DB.Close()

	if command != "serve" {
		logger.Info("command started")
		ctx, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
		defer cancel()
		switch command {
		case "migrate":
			goose.SetLogger(logging.Goose{Logger: logger})
			goose.SetBaseFS(migrations.Files)
			if err := goose.SetDialect("postgres"); err != nil {
				return err
			}

			err = goose.UpContext(ctx, db.DB.DB, ".")
		case "seed":
			err = db.Seed(ctx)
		case "cleanup-sessions":
			err = db.Cleanup(ctx)
		default:
			return fmt.Errorf("unknown command: %s", command)
		}
		if err == nil {
			logger.Info("command completed")
		}
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.QueryTimeout)
	err = db.DB.PingContext(ctx)
	cancel()
	if err != nil {
		return fmt.Errorf("database startup check: %w", err)
	}

	observations := metrics.New()
	api := &httpapi.API{Service: &service.Service{Store: db, Metrics: observations, InviteCode: c.InviteCode, SessionDuration: c.SessionDuration, Faults: service.FaultInjector{Rate: c.LabFaultRate, Delay: c.LabFaultDelay}}, Config: c, Logger: logger}
	server := &http.Server{Addr: ":" + c.Port, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: c.QueryTimeout + 5*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	monitoring := &http.Server{Addr: ":" + c.MetricsPort, Handler: observations.Handler(), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}

	stop, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer done()

	return serve(stop, c.ShutdownTimeout, func() { api.ShuttingDown.Store(true) }, server, monitoring)
}

// Bind every port before serving. A failed metrics bind must not leave a
// partially started application listening on its business port.
func serve(stop context.Context, timeout time.Duration, shuttingDown func(), servers ...*http.Server) error {
	listeners := make([]net.Listener, 0, len(servers))
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	for _, server := range servers {
		listener, err := net.Listen("tcp", server.Addr)
		if err != nil {
			return err
		}
		listeners = append(listeners, listener)
	}
	failures := make(chan error, len(servers))
	for i, server := range servers {
		slog.Info("listening", "address", listeners[i].Addr().String(), "version", version)
		go func() { failures <- server.Serve(listeners[i]) }()
	}
	var serveErr error
	select {
	case err := <-failures:
		if !errors.Is(err, http.ErrServerClosed) {
			serveErr = err
		}
	case <-stop.Done():
	}

	shuttingDown()
	shutdown, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	slog.Info("shutting down")
	results := make(chan error, len(servers))
	for _, server := range servers {
		go func() {
			err := server.Shutdown(shutdown)
			if err != nil {
				_ = server.Close()
			}
			results <- err
		}()
	}
	for range servers {
		serveErr = errors.Join(serveErr, <-results)
	}
	return serveErr
}
