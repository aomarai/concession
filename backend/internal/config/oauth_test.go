package config

import (
	"testing"

	"golang.org/x/oauth2/google"
)

func TestGetGoogleOAuthConfigFields(t *testing.T) {
	cfg := &Config{
		GoogleClientID:     "test-client-id",
		GoogleClientSecret: "test-client-secret",
		GoogleRedirectURL:  "https://example.com/oauth/callback",
	}

	oauthCfg := GetGoogleOAuthConfig(cfg)

	if oauthCfg.ClientID != cfg.GoogleClientID {
		t.Errorf("expected ClientID %q, got %q", cfg.GoogleClientID, oauthCfg.ClientID)
	}
	if oauthCfg.ClientSecret != cfg.GoogleClientSecret {
		t.Errorf("expected ClientSecret %q, got %q", cfg.GoogleClientSecret, oauthCfg.ClientSecret)
	}
	if oauthCfg.RedirectURL != cfg.GoogleRedirectURL {
		t.Errorf("expected RedirectURL %q, got %q", cfg.GoogleRedirectURL, oauthCfg.RedirectURL)
	}
}

func TestGetGoogleOAuthConfigScopes(t *testing.T) {
	cfg := &Config{}
	oauthCfg := GetGoogleOAuthConfig(cfg)

	want := map[string]bool{
		"https://www.googleapis.com/auth/userinfo.profile": true,
		"https://www.googleapis.com/auth/userinfo.email":   true,
	}

	if len(oauthCfg.Scopes) != len(want) {
		t.Fatalf("expected %d scopes, got %d: %v", len(want), len(oauthCfg.Scopes), oauthCfg.Scopes)
	}
	for _, scope := range oauthCfg.Scopes {
		if !want[scope] {
			t.Errorf("unexpected scope: %q", scope)
		}
	}
}

func TestGetGoogleOAuthConfigEndpoint(t *testing.T) {
	cfg := &Config{}
	oauthCfg := GetGoogleOAuthConfig(cfg)

	if oauthCfg.Endpoint != google.Endpoint {
		t.Errorf("expected Endpoint to be google.Endpoint, got %+v", oauthCfg.Endpoint)
	}
}

func TestGetGoogleOAuthConfigPassesThroughEmptyCredentials(t *testing.T) {
	// GetGoogleOAuthConfig doesn't validate its input — an empty Config
	// still produces a usable-looking oauth2.Config, just with empty
	// credentials. Whatever calls this (e.g. Load or a handler) is
	// responsible for catching missing Google OAuth credentials before
	// this point; this function itself won't error or fill in placeholders.
	cfg := &Config{}
	oauthCfg := GetGoogleOAuthConfig(cfg)

	if oauthCfg.ClientID != "" {
		t.Errorf("expected empty ClientID to pass through, got %q", oauthCfg.ClientID)
	}
	if oauthCfg.ClientSecret != "" {
		t.Errorf("expected empty ClientSecret to pass through, got %q", oauthCfg.ClientSecret)
	}
}
