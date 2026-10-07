package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/pressly/goose/v3"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sre_tbank/internal/config"
	"sre_tbank/internal/httpapi"
	"sre_tbank/internal/service"
	"sre_tbank/internal/store"
	"sre_tbank/migrations"
	"syscall"
	"time"
)

var version = "development"

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}

	level := slog.LevelInfo
	_ = level.UnmarshalText([]byte(c.LogLevel))
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	db := store.Open(c.DatabaseURL, c.PoolSize)
	defer db.DB.Close()

	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	if command != "serve" {
		ctx, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
		defer cancel()
		switch command {
		case "migrate":
			goose.SetBaseFS(migrations.Files)
			if err := goose.SetDialect("postgres"); err != nil {
				return err
			}

			return goose.UpContext(ctx, db.DB.DB, ".")
		case "seed":
			return db.Seed(ctx)
		case "cleanup-sessions":
			return db.Cleanup(ctx)
		default:
			return fmt.Errorf("unknown command: %s", command)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.QueryTimeout)
	err = db.DB.PingContext(ctx)
	cancel()
	if err != nil {
		return fmt.Errorf("database startup check: %w", err)
	}

	api := &httpapi.API{Service: &service.Service{Store: db, InviteCode: c.InviteCode, SessionDuration: c.SessionDuration}, Config: c, Logger: logger}
	server := &http.Server{Addr: ":" + c.Port, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: c.QueryTimeout + 5*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}

	stop, done := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer done()

	failures := make(chan error, 1)
	go func() {
		logger.Info("listening", "port", c.Port, "version", version)
		failures <- server.ListenAndServe()
	}()

	select {
	case err := <-failures:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}

		return nil
	case <-stop.Done():
	}

	api.ShuttingDown.Store(true)
	shutdown, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
	defer cancel()

	logger.Info("shutting down")
	if err := server.Shutdown(shutdown); err != nil {
		_ = server.Close()
		return err
	}

	return nil
}
