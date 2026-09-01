package config

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func assertNoError(t *testing.T, got error) {
	t.Helper()
	if got != nil {
		t.Fatalf("got an error but didn't want one: %s", got)
	}
}

func assertDeepEqual(t *testing.T, got, want interface{}) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEnv(t *testing.T) {
	const (
		testFpanDatabaseUrl      = "postgres://fpan:fpan@localhost:5432/fpan"
		testFpanStoragePath      = "./files"
		testFpanOidcIssuer       = "https://auth.example.com/.well-known/openid-configuration"
		testFpanOidcClientID     = "oidc_client_id_xxxxxx"
		testFpanOidcClientSecret = "oidc_client_secret_yyyyyy"
		testFpanOidcRedirectUrl  = "https://fpan.test.local/auth/oidc/callback"
		testFpanListenAddr       = ":11011"
	)

	t.Run("load with optional env", func(t *testing.T) {
		t.Setenv(fpanDatabaseUrlEnv, testFpanDatabaseUrl)
		t.Setenv(fpanStoragePathEnv, testFpanStoragePath)
		t.Setenv(fpanAuthModeEnv, string(AuthModeOIDC))
		t.Setenv(fpanOidcIssuerEnv, testFpanOidcIssuer)
		t.Setenv(fpanOidcClientIDEnv, testFpanOidcClientID)
		t.Setenv(fpanOidcClientSecretEnv, testFpanOidcClientSecret)
		t.Setenv(fpanOidcRedirectUrlEnv, testFpanOidcRedirectUrl)
		t.Setenv(fpanListenAddr, testFpanListenAddr)

		got, err := loadEnv()
		assertNoError(t, err)

		want := &Env{
			DatabaseUrl:       testFpanDatabaseUrl,
			StoragePath:       testFpanStoragePath,
			AuthMode:          AuthModeOIDC,
			OidcIssuer:        testFpanOidcIssuer,
			OidcClientID:      testFpanOidcClientID,
			OidcClientSecret:  testFpanOidcClientSecret,
			OidcRedirectUrl:   testFpanOidcRedirectUrl,
			ListenAddr:        testFpanListenAddr,
			GCInterval:        defaultGCInterval,
			GCGracePeriod:     defaultGCGracePeriod,
			GCBatchSize:       defaultGCBatchSize,
			ReadHeaderTimeout: defaultReadHeaderTimeout,
			IdleTimeout:       defaultIdleTimeout,
			ShutdownTimeout:   defaultShutdownTimeout,
		}

		assertDeepEqual(t, got, want)
	})

	t.Run("load without optional env", func(t *testing.T) {
		t.Setenv(fpanDatabaseUrlEnv, testFpanDatabaseUrl)
		unsetEnv(t, fpanAuthModeEnv)
		t.Setenv(fpanOidcIssuerEnv, testFpanOidcIssuer)
		t.Setenv(fpanOidcClientIDEnv, testFpanOidcClientID)
		t.Setenv(fpanOidcClientSecretEnv, testFpanOidcClientSecret)
		t.Setenv(fpanOidcRedirectUrlEnv, testFpanOidcRedirectUrl)

		got, err := loadEnv()
		assertNoError(t, err)

		want := &Env{
			DatabaseUrl:       testFpanDatabaseUrl,
			StoragePath:       defaultStoragePath,
			AuthMode:          AuthModeOIDC,
			OidcIssuer:        testFpanOidcIssuer,
			OidcClientID:      testFpanOidcClientID,
			OidcClientSecret:  testFpanOidcClientSecret,
			OidcRedirectUrl:   testFpanOidcRedirectUrl,
			ListenAddr:        defaultListenAddr,
			GCInterval:        defaultGCInterval,
			GCGracePeriod:     defaultGCGracePeriod,
			GCBatchSize:       defaultGCBatchSize,
			ReadHeaderTimeout: defaultReadHeaderTimeout,
			IdleTimeout:       defaultIdleTimeout,
			ShutdownTimeout:   defaultShutdownTimeout,
		}

		assertDeepEqual(t, got, want)
	})
}

func TestEnvLoadsMockAuthenticationWithoutOIDC(t *testing.T) {
	t.Setenv(fpanDatabaseUrlEnv, "postgres://fpan:fpan@localhost:5432/fpan")
	t.Setenv(fpanAuthModeEnv, string(AuthModeMock))
	t.Setenv(fpanListenAddr, "127.0.0.1:6313")
	for _, key := range []string{fpanOidcIssuerEnv, fpanOidcClientIDEnv, fpanOidcClientSecretEnv, fpanOidcRedirectUrlEnv} {
		unsetEnv(t, key)
	}

	got, err := loadEnv()
	assertNoError(t, err)
	if got.AuthMode != AuthModeMock || got.OidcIssuer != "" || got.OidcClientID != "" || got.OidcClientSecret != "" || got.OidcRedirectUrl != "" {
		t.Fatalf("mock authentication settings = %#v", got)
	}
}

func TestEnvDefaultAuthenticationRequiresOIDC(t *testing.T) {
	t.Setenv(fpanDatabaseUrlEnv, "postgres://fpan:fpan@localhost:5432/fpan")
	unsetEnv(t, fpanAuthModeEnv)
	for _, key := range []string{fpanOidcIssuerEnv, fpanOidcClientIDEnv, fpanOidcClientSecretEnv, fpanOidcRedirectUrlEnv} {
		unsetEnv(t, key)
	}
	if _, err := loadEnv(); err == nil {
		t.Fatal("loadEnv() accepted default OIDC mode without OIDC settings")
	}
}

func TestEnvValidatesAuthenticationMode(t *testing.T) {
	t.Setenv(fpanDatabaseUrlEnv, "postgres://fpan:fpan@localhost:5432/fpan")
	t.Setenv(fpanAuthModeEnv, "invalid")
	if _, err := loadEnv(); err == nil {
		t.Fatal("loadEnv() accepted an invalid authentication mode")
	}
}

func TestEnvRestrictsMockAuthenticationToLoopback(t *testing.T) {
	for _, address := range []string{"localhost:6313", "127.0.0.1:6313", "127.42.0.1:6313", "[::1]:6313"} {
		t.Run("accept_"+address, func(t *testing.T) {
			t.Setenv(fpanDatabaseUrlEnv, "postgres://fpan:fpan@localhost:5432/fpan")
			t.Setenv(fpanAuthModeEnv, string(AuthModeMock))
			t.Setenv(fpanListenAddr, address)
			if _, err := loadEnv(); err != nil {
				t.Fatalf("loadEnv() rejected loopback address %q: %v", address, err)
			}
		})
	}
	for _, address := range []string{":6313", "0.0.0.0:6313", "[::]:6313", "192.0.2.1:6313"} {
		t.Run("reject_"+address, func(t *testing.T) {
			t.Setenv(fpanDatabaseUrlEnv, "postgres://fpan:fpan@localhost:5432/fpan")
			t.Setenv(fpanAuthModeEnv, string(AuthModeMock))
			t.Setenv(fpanListenAddr, address)
			if _, err := loadEnv(); err == nil {
				t.Fatalf("loadEnv() accepted non-loopback address %q", address)
			}
		})
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, exists := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv(key, value)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func TestEnvParsesRuntimeSettings(t *testing.T) {
	t.Setenv(fpanDatabaseUrlEnv, "postgres://fpan:fpan@localhost:5432/fpan")
	t.Setenv(fpanAuthModeEnv, string(AuthModeOIDC))
	t.Setenv(fpanOidcIssuerEnv, "https://auth.example.com")
	t.Setenv(fpanOidcClientIDEnv, "client")
	t.Setenv(fpanOidcClientSecretEnv, "secret")
	t.Setenv(fpanOidcRedirectUrlEnv, "https://fpan.example/callback")
	t.Setenv(fpanGCIntervalEnv, "30m")
	t.Setenv(fpanGCGracePeriodEnv, "48h")
	t.Setenv(fpanGCBatchSizeEnv, "25")
	t.Setenv(fpanReadHeaderTimeoutEnv, "2s")
	t.Setenv(fpanIdleTimeoutEnv, "0")
	t.Setenv(fpanShutdownTimeoutEnv, "15s")

	got, err := loadEnv()
	assertNoError(t, err)
	if got.GCInterval != 30*time.Minute || got.GCGracePeriod != 48*time.Hour || got.GCBatchSize != 25 ||
		got.ReadHeaderTimeout != 2*time.Second || got.IdleTimeout != 0 || got.ShutdownTimeout != 15*time.Second {
		t.Fatalf("runtime settings = %#v", got)
	}
}

func TestEnvRejectsInvalidRuntimeSettings(t *testing.T) {
	t.Setenv(fpanDatabaseUrlEnv, "postgres://fpan:fpan@localhost:5432/fpan")
	t.Setenv(fpanAuthModeEnv, string(AuthModeOIDC))
	t.Setenv(fpanOidcIssuerEnv, "https://auth.example.com")
	t.Setenv(fpanOidcClientIDEnv, "client")
	t.Setenv(fpanOidcClientSecretEnv, "secret")
	t.Setenv(fpanOidcRedirectUrlEnv, "https://fpan.example/callback")
	for key, value := range map[string]string{
		fpanGCIntervalEnv:        "0",
		fpanGCGracePeriodEnv:     "invalid",
		fpanGCBatchSizeEnv:       "0",
		fpanReadHeaderTimeoutEnv: "-1s",
		fpanShutdownTimeoutEnv:   "0",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := loadEnv(); err == nil {
				t.Fatalf("loadEnv() accepted %s=%s", key, value)
			}
		})
	}
}
