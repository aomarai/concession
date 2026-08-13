package config

import (
	"context"
	"os"
	"testing"
)

// allConfigEnvKeys lists every env var Config reads.
var allConfigEnvKeys = []string{
	"DB_DRIVER", "DB_HOST", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_PORT",
	"DB_PATH", "ENV", "JWT_SECRET", "PORT", "COOKIE_SECURE",
	"GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET", "GOOGLE_REDIRECT_URL",
}

// unsetAllConfigEnv fully unsets every Config-related env var and restores
// whatever was there once the test finishes.
//
// This uses os.Unsetenv rather than t.Setenv("KEY", "") deliberately:
// envconfig's `default=...` tag only applies when a variable is completely
// absent (os.LookupEnv returns false), not when it's present-but-empty. A
// t.Setenv("KEY", "") would count as "set" and suppress the default, which
// would break the "defaults apply" tests below.
//
// Caveat: Load() calls godotenv.Load(), which reads a ".env" file from the
// current working directory (which `go test` sets to this package's own
// directory, internal/config) and only fills in variables that AREN'T
// already present in the OS environment. So: values set via t.Setenv in a
// test always win (they're already "present" before Load() runs), but if
// you ever add a real .env file directly inside internal/config/, the
// "defaults apply" tests below could pick up values from it instead of
// getting the code's actual defaults. Not an issue as long as .env lives
// at the repo root instead.
func unsetAllConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range allConfigEnvKeys {
		original, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("failed to unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if existed {
				os.Setenv(key, original)
			} else {
				os.Unsetenv(key)
			}
		})
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	unsetAllConfigEnv(t)

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DBDriver != "sqlite" {
		t.Errorf("expected DBDriver default %q, got %q", "sqlite", cfg.DBDriver)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected Environment default %q, got %q", "development", cfg.Environment)
	}
	if !cfg.CookieSecure {
		t.Error("expected CookieSecure to default to true")
	}
}

func TestLoadFieldsWithoutDefaultsAreEmpty(t *testing.T) {
	unsetAllConfigEnv(t)

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields := map[string]string{
		"DBHost":             cfg.DBHost,
		"DBUser":             cfg.DBUser,
		"DBPassword":         cfg.DBPassword,
		"DBName":             cfg.DBName,
		"DBPort":             cfg.DBPort,
		"DBPath":             cfg.DBPath,
		"JWTSecret":          cfg.JWTSecret,
		"Port":               cfg.Port,
		"GoogleClientID":     cfg.GoogleClientID,
		"GoogleClientSecret": cfg.GoogleClientSecret,
		"GoogleRedirectURL":  cfg.GoogleRedirectURL,
	}
	for name, val := range fields {
		if val != "" {
			t.Errorf("expected %s to be empty with no env var and no default, got %q", name, val)
		}
	}
}

func TestLoadReadsEnvVars(t *testing.T) {
	unsetAllConfigEnv(t)

	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_USER", "concession")
	t.Setenv("DB_PASSWORD", "hunter2")
	t.Setenv("DB_NAME", "concession_prod")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("ENV", "production")
	t.Setenv("JWT_SECRET", "top-secret")
	t.Setenv("PORT", "8080")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "https://example.com/callback")

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"DBDriver", cfg.DBDriver, "postgres"},
		{"DBHost", cfg.DBHost, "db.example.com"},
		{"DBUser", cfg.DBUser, "concession"},
		{"DBPassword", cfg.DBPassword, "hunter2"},
		{"DBName", cfg.DBName, "concession_prod"},
		{"DBPort", cfg.DBPort, "5432"},
		{"Environment", cfg.Environment, "production"},
		{"JWTSecret", cfg.JWTSecret, "top-secret"},
		{"Port", cfg.Port, "8080"},
		{"GoogleClientID", cfg.GoogleClientID, "client-id"},
		{"GoogleClientSecret", cfg.GoogleClientSecret, "client-secret"},
		{"GoogleRedirectURL", cfg.GoogleRedirectURL, "https://example.com/callback"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("expected %s to be %q, got %q", tc.name, tc.want, tc.got)
		}
	}
	if cfg.CookieSecure {
		t.Error("expected CookieSecure to be false when COOKIE_SECURE=false")
	}
}

func TestLoadCookieSecureBooleanParsing(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"true", "true", true},
		{"false", "false", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unsetAllConfigEnv(t)
			t.Setenv("COOKIE_SECURE", tc.value)

			cfg, err := Load(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.CookieSecure != tc.want {
				t.Errorf("expected CookieSecure=%v for COOKIE_SECURE=%q, got %v", tc.want, tc.value, cfg.CookieSecure)
			}
		})
	}
}
