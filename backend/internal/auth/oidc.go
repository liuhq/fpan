package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const oidcStateLifetime = 5 * time.Minute

type OIDC struct {
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier
	states       sync.Map
}

type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type oidcState struct {
	Nonce     string
	ExpiresAt time.Time
}

func NewOIDC(ctx context.Context, cfg OIDCConfig) (*OIDC, error) {
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("create oidc provider: %w", err)
	}

	return &OIDC{
		oauth2Config: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes: []string{
				oidc.ScopeOpenID,
			},
		},

		verifier: provider.Verifier(&oidc.Config{
			ClientID: cfg.ClientID,
		}),
	}, nil
}

func randomString() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate random data: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func (o *OIDC) LoginURL() (string, error) {
	state, err := randomString()
	if err != nil {
		return "", err
	}

	nonce, err := randomString()
	if err != nil {
		return "", err
	}

	o.states.Store(state, oidcState{
		Nonce:     nonce,
		ExpiresAt: time.Now().Add(oidcStateLifetime),
	})

	return o.oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce)), nil
}

func (o *OIDC) consumeState(state string) (oidcState, error) {
	value, ok := o.states.LoadAndDelete(state)
	if !ok {
		return oidcState{}, errors.New("invalid oidc state")
	}

	data := value.(oidcState)

	if time.Now().After(data.ExpiresAt) {
		return oidcState{}, errors.New("oidc state expired")
	}

	return data, nil
}

func (o *OIDC) Authenticate(ctx context.Context, code, state string) error {
	stateData, err := o.consumeState(state)
	if err != nil {
		return err
	}

	token, err := o.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("exchange authorization code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return errors.New("missing id_token")
	}

	idToken, err := o.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return fmt.Errorf("verify id token: %w", err)
	}

	if idToken.Nonce != stateData.Nonce {
		return errors.New("invalid oidc nonce")
	}

	return nil
}
