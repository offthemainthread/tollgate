package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/offthemainline/tollgate/ingest-go/internal/api"
	"github.com/offthemainline/tollgate/ingest-go/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	svr := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewRouter(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting server", "addr", svr.Addr)
		errCh <- svr.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		logger.Error("server error", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		logger.Info("shutting down server")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := svr.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
		os.Exit(1)
	}
}
