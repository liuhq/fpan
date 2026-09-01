package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/liuhq/fpan/internal/auth"
	"github.com/liuhq/fpan/internal/config"
	"github.com/liuhq/fpan/internal/database"
	"github.com/liuhq/fpan/internal/files"
	"github.com/liuhq/fpan/internal/httpapi"
	"github.com/liuhq/fpan/internal/shares"
	"github.com/liuhq/fpan/internal/storage/filesystem"
)

const startupTimeout = 30 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	env, err := config.Load()
	if err != nil {
		return err
	}

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), startupTimeout)
	defer cancelStartup()

	db, err := database.Open(env.DatabaseUrl)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("ERROR: close database: %v", closeErr)
		}
	}()
	if err := db.Ping(startupCtx); err != nil {
		return fmt.Errorf("check database: %w", err)
	}

	if err := db.Migrate(); err != nil {
		return err
	}

	store, err := filesystem.New(env.StoragePath)
	if err != nil {
		return err
	}
	fileService, err := files.New(db, store)
	if err != nil {
		return err
	}
	shareService, err := shares.New(db, fileService)
	if err != nil {
		return err
	}
	var oidcClient httpapi.OIDC
	if env.AuthMode == config.AuthModeMock {
		log.Printf("WARN: mock authentication is enabled; use it only for local development")
		oidcClient = auth.NewMockOIDC()
	} else {
		oidcClient, err = auth.NewOIDC(startupCtx, auth.OIDCConfig{
			Issuer: env.OidcIssuer, ClientID: env.OidcClientID, ClientSecret: env.OidcClientSecret, RedirectURL: env.OidcRedirectUrl,
		})
		if err != nil {
			return err
		}
	}
	r, err := httpapi.NewRouter(httpapi.RouterConfig{
		Repository:    db,
		Files:         fileService,
		Shares:        shareService,
		OIDC:          oidcClient,
		Sessions:      auth.NewSessions(),
		SecureCookies: env.AuthMode == config.AuthModeOIDC,
		Ready: func(ctx context.Context) error {
			return errors.Join(db.Ping(ctx), store.Ready(ctx))
		},
	})
	if err != nil {
		return err
	}

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	gcCtx, cancelGC := context.WithCancel(appCtx)
	defer cancelGC()
	gcDone := make(chan error, 1)
	go func() {
		gcDone <- fileService.RunGarbageCollector(gcCtx, env.GCInterval, files.GCOptions{
			GracePeriod: env.GCGracePeriod,
			BatchSize:   env.GCBatchSize,
		}, log.Printf)
	}()

	srv := &http.Server{
		Addr:              env.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: env.ReadHeaderTimeout,
		IdleTimeout:       env.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("INFO: listening at %s", srv.Addr)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		cancelGC()
		<-gcDone
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-appCtx.Done():
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), env.ShutdownTimeout)
		shutdownErr := srv.Shutdown(shutdownCtx)
		cancelShutdown()
		cancelGC()
		gcErr := <-gcDone
		return errors.Join(shutdownErr, gcErr)
	}
}
