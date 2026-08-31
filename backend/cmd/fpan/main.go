package main

import (
	"context"
	"log"
	"net/http"

	"github.com/liuhq/fpan/internal/auth"
	"github.com/liuhq/fpan/internal/config"
	"github.com/liuhq/fpan/internal/database"
	"github.com/liuhq/fpan/internal/files"
	"github.com/liuhq/fpan/internal/httpapi"
	"github.com/liuhq/fpan/internal/storage/filesystem"
)

func main() {
	env, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := database.Open(env.DatabaseUrl)
	if err != nil {
		log.Fatal(err)
	}

	if err = db.Migrate(); err != nil {
		log.Fatal(err)
	}

	store, err := filesystem.New(env.StoragePath)
	if err != nil {
		log.Fatal(err)
	}
	fileService, err := files.New(db, store)
	if err != nil {
		log.Fatal(err)
	}
	oidcClient, err := auth.NewOIDC(context.Background(), auth.OIDCConfig{
		Issuer: env.OidcIssuer, ClientID: env.OidcClientID, ClientSecret: env.OidcClientSecret, RedirectURL: env.OidcRedirectUrl,
	})
	if err != nil {
		log.Fatal(err)
	}
	r, err := httpapi.NewRouter(httpapi.RouterConfig{
		Repository: db, Files: fileService, OIDC: oidcClient, Sessions: auth.NewSessions(),
	})
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:    env.ListenAddr,
		Handler: r,
	}

	err = srv.ListenAndServe()
	log.Printf("INFO: listening at %s", srv.Addr)
	if err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
