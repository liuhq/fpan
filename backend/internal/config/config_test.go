package config

import (
	"reflect"
	"testing"
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
		testFpanDatabaseUrl  = "postgres://fpan:fpan@localhost:5432/fpan"
		testFpanStoragePath  = "./files"
		testFpanOidcIssuer   = "https://auth.example.com/.well-known/openid-configuration"
		testFpanOidcClientID = "oidc_client_id_xxxxxx"
		testFpanListenAddr   = ":11011"
	)

	t.Run("load with optional env", func(t *testing.T) {
		t.Setenv(fpanDatabaseUrlEnv, testFpanDatabaseUrl)
		t.Setenv(fpanStoragePathEnv, testFpanStoragePath)
		t.Setenv(fpanOidcIssuerEnv, testFpanOidcIssuer)
		t.Setenv(fpanOidcClientIDEnv, testFpanOidcClientID)
		t.Setenv(fpanListenAddr, testFpanListenAddr)

		got, err := loadEnv()
		assertNoError(t, err)

		want := &Env{
			DatabaseUrl:  testFpanDatabaseUrl,
			StoragePath:  testFpanStoragePath,
			OidcIssuer:   testFpanOidcIssuer,
			OidcClientID: testFpanOidcClientID,
			ListenAddr:   testFpanListenAddr,
		}

		assertDeepEqual(t, got, want)
	})

	t.Run("load without optional env", func(t *testing.T) {
		t.Setenv(fpanDatabaseUrlEnv, testFpanDatabaseUrl)
		t.Setenv(fpanOidcIssuerEnv, testFpanOidcIssuer)
		t.Setenv(fpanOidcClientIDEnv, testFpanOidcClientID)

		got, err := loadEnv()
		assertNoError(t, err)

		want := &Env{
			DatabaseUrl:  testFpanDatabaseUrl,
			StoragePath:  defaultStoragePath,
			OidcIssuer:   testFpanOidcIssuer,
			OidcClientID: testFpanOidcClientID,
			ListenAddr:   defaultListenAddr,
		}

		assertDeepEqual(t, got, want)
	})
}
