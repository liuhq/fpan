package auth

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMockOIDCAuthenticatesOneTimeCallback(t *testing.T) {
	mock := NewMockOIDC()
	callback := mockCallback(t, mock)
	code, state := callback.Query().Get("code"), callback.Query().Get("state")
	if code != mockAuthorizationCode || state == "" {
		t.Fatalf("mock callback query = %q", callback.RawQuery)
	}
	if err := mock.Authenticate(context.Background(), code, state); err != nil {
		t.Fatalf("authenticate mock callback: %v", err)
	}
	if err := mock.Authenticate(context.Background(), code, state); err == nil {
		t.Fatal("mock authentication accepted a replayed state")
	}
}

func TestMockOIDCRejectsInvalidAndExpiredCallbacks(t *testing.T) {
	mock := NewMockOIDC()
	callback := mockCallback(t, mock)
	state := callback.Query().Get("state")
	if err := mock.Authenticate(context.Background(), "wrong", state); err == nil {
		t.Fatal("mock authentication accepted an invalid code")
	}
	if err := mock.Authenticate(context.Background(), mockAuthorizationCode, state); err == nil {
		t.Fatal("invalid-code attempt did not consume the state")
	}

	expiredState := "expired"
	mock.states.values.Store(expiredState, oidcState{ExpiresAt: time.Now().Add(-time.Second)})
	if err := mock.Authenticate(context.Background(), mockAuthorizationCode, expiredState); err == nil {
		t.Fatal("mock authentication accepted an expired state")
	}
	if err := mock.Authenticate(context.Background(), mockAuthorizationCode, "missing"); err == nil {
		t.Fatal("mock authentication accepted an unknown state")
	}
}

func TestMockOIDCIssuesUniqueStates(t *testing.T) {
	first := mockCallback(t, NewMockOIDC()).Query().Get("state")
	mock := NewMockOIDC()
	second := mockCallback(t, mock).Query().Get("state")
	third := mockCallback(t, mock).Query().Get("state")
	if first == "" || second == "" || third == "" || first == second || second == third || first == third {
		t.Fatalf("mock states are not unique: %q, %q, %q", first, second, third)
	}
}

func mockCallback(t *testing.T, mock *MockOIDC) *url.URL {
	t.Helper()
	value, err := mock.LoginURL()
	if err != nil {
		t.Fatal(err)
	}
	callback, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	if callback.Path != "/api/v1/auth/callback" || !strings.HasPrefix(value, "/") {
		t.Fatalf("mock login URL = %q", value)
	}
	return callback
}
