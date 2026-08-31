package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	fpanDatabaseUrlEnv       = "FPAN_DATABASE_URL"
	fpanStoragePathEnv       = "FPAN_STORAGE_PATH"
	fpanOidcIssuerEnv        = "FPAN_OIDC_ISSUER"
	fpanOidcClientIDEnv      = "FPAN_OIDC_CLIENT_ID"
	fpanOidcClientSecretEnv  = "FPAN_OIDC_CLIENT_SECRET"
	fpanOidcRedirectUrlEnv   = "FPAN_OIDC_REDIRECT_URL"
	fpanListenAddr           = "FPAN_LISTEN_ADDR"
	fpanGCIntervalEnv        = "FPAN_GC_INTERVAL"
	fpanGCGracePeriodEnv     = "FPAN_GC_GRACE_PERIOD"
	fpanGCBatchSizeEnv       = "FPAN_GC_BATCH_SIZE"
	fpanReadHeaderTimeoutEnv = "FPAN_HTTP_READ_HEADER_TIMEOUT"
	fpanIdleTimeoutEnv       = "FPAN_HTTP_IDLE_TIMEOUT"
	fpanShutdownTimeoutEnv   = "FPAN_HTTP_SHUTDOWN_TIMEOUT"
)

const (
	defaultListenAddr        = ":6313"
	defaultStoragePath       = "./storage"
	defaultGCInterval        = time.Hour
	defaultGCGracePeriod     = 24 * time.Hour
	defaultGCBatchSize       = 100
	defaultReadHeaderTimeout = 5 * time.Second
	defaultIdleTimeout       = time.Minute
	defaultShutdownTimeout   = 10 * time.Second
)

func Load() (*Env, error) {
	err := godotenv.Load()
	if err != nil {
		log.Printf("WARN: %s", err)
	}

	return loadEnv()
}

type Env struct {
	DatabaseUrl       string
	StoragePath       string
	OidcIssuer        string
	OidcClientID      string
	OidcClientSecret  string
	OidcRedirectUrl   string
	ListenAddr        string
	GCInterval        time.Duration
	GCGracePeriod     time.Duration
	GCBatchSize       int
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
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
	gcInterval, err := optionalDuration(fpanGCIntervalEnv, defaultGCInterval, false)
	if err != nil {
		return nil, err
	}
	gcGracePeriod, err := optionalDuration(fpanGCGracePeriodEnv, defaultGCGracePeriod, false)
	if err != nil {
		return nil, err
	}
	gcBatchSize, err := optionalPositiveInt(fpanGCBatchSizeEnv, defaultGCBatchSize)
	if err != nil {
		return nil, err
	}
	readHeaderTimeout, err := optionalDuration(fpanReadHeaderTimeoutEnv, defaultReadHeaderTimeout, true)
	if err != nil {
		return nil, err
	}
	idleTimeout, err := optionalDuration(fpanIdleTimeoutEnv, defaultIdleTimeout, true)
	if err != nil {
		return nil, err
	}
	shutdownTimeout, err := optionalDuration(fpanShutdownTimeoutEnv, defaultShutdownTimeout, false)
	if err != nil {
		return nil, err
	}
	return &Env{
		DatabaseUrl:       databaseUrl,
		StoragePath:       storagePath,
		OidcIssuer:        oidcIssuer,
		OidcClientID:      oidcClientID,
		OidcClientSecret:  oidcClientSecret,
		OidcRedirectUrl:   oidcRedirectUrl,
		ListenAddr:        listenAddr,
		GCInterval:        gcInterval,
		GCGracePeriod:     gcGracePeriod,
		GCBatchSize:       gcBatchSize,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
	}, nil
}

func optionalDuration(key string, fallback time.Duration, allowZero bool) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || (!allowZero && duration <= 0) || (allowZero && duration < 0) {
		return 0, fmt.Errorf("environment variable %q must be a valid %s duration", key, map[bool]string{true: "non-negative", false: "positive"}[allowZero])
	}
	return duration, nil
}

func optionalPositiveInt(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("environment variable %q must be a positive integer", key)
	}
	return parsed, nil
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
