package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

const (
	fpanDatabaseUrlEnv      = "FPAN_DATABASE_URL"
	fpanStoragePathEnv      = "FPAN_STORAGE_PATH"
	fpanOidcIssuerEnv       = "FPAN_OIDC_ISSUER"
	fpanOidcClientIDEnv     = "FPAN_OIDC_CLIENT_ID"
	fpanOidcClientSecretEnv = "FPAN_OIDC_CLIENT_SECRET"
	fpanOidcRedirectUrlEnv  = "FPAN_OIDC_REDIRECT_URL"
	fpanListenAddr          = "FPAN_LISTEN_ADDR"
)

const (
	defaultListenAddr  = ":6313"
	defaultStoragePath = "./storage"
)

func Load() (*Env, error) {
	err := godotenv.Load()
	if err != nil {
		log.Printf("WARN: %s", err)
	}

	return loadEnv()
}

type Env struct {
	DatabaseUrl      string
	StoragePath      string
	OidcIssuer       string
	OidcClientID     string
	OidcClientSecret string
	OidcRedirectUrl  string
	ListenAddr       string
}

func loadEnv() (*Env, error) {
	databaseUrl, err := requiredEnv(fpanDatabaseUrlEnv)
	if err != nil {
		return nil, err
	}
	storagePath := optionalEnv(fpanStoragePathEnv, defaultStoragePath)
	oidcIssuer, err := requiredEnv(fpanOidcIssuerEnv)
	if err != nil {
		return nil, err
	}
	oidcClientID, err := requiredEnv(fpanOidcClientIDEnv)
	if err != nil {
		return nil, err
	}
	oidcClientSecret, err := requiredEnv(fpanOidcClientSecretEnv)
	if err != nil {
		return nil, err
	}
	oidcRedirectUrl, err := requiredEnv(fpanOidcRedirectUrlEnv)
	if err != nil {
		return nil, err
	}
	listenAddr := optionalEnv(fpanListenAddr, defaultListenAddr)
	return &Env{
		DatabaseUrl:      databaseUrl,
		StoragePath:      storagePath,
		OidcIssuer:       oidcIssuer,
		OidcClientID:     oidcClientID,
		OidcClientSecret: oidcClientSecret,
		OidcRedirectUrl:  oidcRedirectUrl,
		ListenAddr:       listenAddr,
	}, nil
}

func optionalEnv(key, fallback string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return val
}

func requiredEnv(key string) (string, error) {
	val, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("environment variable %q is required", key)
	}
	return val, nil
}
