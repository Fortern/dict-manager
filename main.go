package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dict-manager/dictionary"
	"dict-manager/web"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	if err := run(context.Background()); err != nil {
		slog.Error("dictionary server stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	db, err := sql.Open("sqlite3", "dict.db")
	if err != nil {
		return fmt.Errorf("open dictionary database: %w", err)
	}
	defer db.Close()

	catalog := dictionary.NewCatalog(db)
	if err := catalog.InitSchema(ctx); err != nil {
		return fmt.Errorf("initialize dictionary database: %w", err)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:              ":8080",
		Handler:           web.New(catalog, slog.Default()).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	slog.Info("starting dictionary server", "addr", server.Addr)
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		return nil
	}
}
