// Command podora runs the Podora backend server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kayamii/Podora/backend/internal/api"
	"github.com/Kayamii/Podora/backend/internal/auth"
	"github.com/Kayamii/Podora/backend/internal/config"
	"github.com/Kayamii/Podora/backend/internal/helm"
	"github.com/Kayamii/Podora/backend/internal/kubernetes"
	"github.com/Kayamii/Podora/backend/internal/resources"
	"github.com/Kayamii/Podora/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("podora exited", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()

	kube, err := kubernetes.New(cfg.Kubeconfig)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cluster := resources.NewService(kube)
	go cluster.RunSampler(ctx)

	hm, err := helm.New(cfg.Kubeconfig)
	if err != nil {
		return err
	}

	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	accounts := auth.New(db)
	token, err := accounts.PrepareSetup(ctx, cfg.SetupToken)
	if err != nil {
		return err
	}
	if token != "" {
		// Printed plainly so it stands out in container logs.
		fmt.Fprintf(os.Stderr, "\n  No Podora accounts exist yet.\n"+
			"  Open Podora and enter this setup token to create the first administrator:\n\n"+
			"      %s\n\n", token)
	}
	go housekeeping(ctx, accounts, db)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewRouter(api.Deps{Cluster: cluster, Helm: hm, Auth: accounts, Audit: db}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("podora listening", "addr", cfg.Addr, "version", api.Version)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// auditRetention is how long audit entries are kept.
const auditRetention = 180 * 24 * time.Hour

// housekeeping removes expired sessions and old audit entries, at start and
// then hourly.
func housekeeping(ctx context.Context, accounts *auth.Service, db *store.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := accounts.CleanUp(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("removing expired sessions", "error", err)
		}
		if err := db.PruneAudit(ctx, time.Now().Add(-auditRetention)); err != nil && ctx.Err() == nil {
			slog.Warn("pruning audit log", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
