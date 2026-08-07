package auth

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB spins up an in-memory SQLite DB and migrates the Session
// model. Each call returns a fresh, isolated DB so tests don't leak state
// into one another.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}

	if err := db.AutoMigrate(&domain.Session{}); err != nil {
		t.Fatalf("failed to migrate domain.Session: %v", err)
	}

	return db
}

func TestGenerateSessionToken(t *testing.T) {
	t.Run("returns no error", func(t *testing.T) {
		if _, err := GenerateSessionToken(); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("returns a non-empty token", func(t *testing.T) {
		token, err := GenerateSessionToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if token == "" {
			t.Fatal("expected non-empty token")
		}
	})

	t.Run("token is valid URL-safe base64 of 32 bytes", func(t *testing.T) {
		token, err := GenerateSessionToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		decoded, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil {
			t.Fatalf("token is not valid RawURLEncoding base64: %v", err)
		}
		if len(decoded) != 32 {
			t.Fatalf("expected decoded token to be 32 bytes, got %d", len(decoded))
		}
	})

	t.Run("generates unique tokens across calls", func(t *testing.T) {
		seen := make(map[string]bool)
		const n = 1000
		for i := 0; i < n; i++ {
			token, err := GenerateSessionToken()
			if err != nil {
				t.Fatalf("unexpected error on iteration %d: %v", i, err)
			}
			if seen[token] {
				t.Fatalf("duplicate token generated: %s", token)
			}
			seen[token] = true
		}
	})
}

func TestHashToken(t *testing.T) {
	t.Run("is deterministic for the same input", func(t *testing.T) {
		token := "some-raw-token-value"
		h1 := HashToken(token)
		h2 := HashToken(token)
		if h1 != h2 {
			t.Fatalf("expected identical hashes, got %s and %s", h1, h2)
		}
	})

	t.Run("produces different hashes for different inputs", func(t *testing.T) {
		h1 := HashToken("token-a")
		h2 := HashToken("token-b")
		if h1 == h2 {
			t.Fatal("expected different hashes for different inputs")
		}
	})

	t.Run("returns a 64-character hex string (sha256)", func(t *testing.T) {
		h := HashToken("anything")
		if len(h) != 64 {
			t.Fatalf("expected 64 hex chars, got %d (%s)", len(h), h)
		}
		for _, c := range h {
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
			if !isHex {
				t.Fatalf("hash contains non-hex character: %q in %s", c, h)
			}
		}
	})

	t.Run("handles empty string input", func(t *testing.T) {
		h := HashToken("")
		if len(h) != 64 {
			t.Fatalf("expected 64 hex chars for empty input, got %d", len(h))
		}
	})
}

func TestCreateSession(t *testing.T) {
	t.Run("creates a session and returns a raw token", func(t *testing.T) {
		db := setupTestDB(t)
		userID := uuid.New()

		rawToken, err := CreateSession(context.Background(), db, userID, "test-agent", "127.0.0.1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rawToken == "" {
			t.Fatal("expected non-empty raw token")
		}

		var stored domain.Session
		if err := db.Where("user_id = ?", userID).First(&stored).Error; err != nil {
			t.Fatalf("expected session to be persisted: %v", err)
		}

		if stored.TokenHash != HashToken(rawToken) {
			t.Fatal("stored token hash does not match hash of returned raw token")
		}
		if stored.TokenHash == rawToken {
			t.Fatal("raw token must not be stored directly in the database")
		}
	})

	t.Run("stores user agent and IP address", func(t *testing.T) {
		db := setupTestDB(t)
		userID := uuid.New()

		_, err := CreateSession(context.Background(), db, userID, "Mozilla/5.0", "192.168.1.1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var stored domain.Session
		if err := db.Where("user_id = ?", userID).First(&stored).Error; err != nil {
			t.Fatalf("expected session to be persisted: %v", err)
		}
		if stored.UserAgent != "Mozilla/5.0" {
			t.Fatalf("expected UserAgent %q, got %q", "Mozilla/5.0", stored.UserAgent)
		}
		if stored.IPAddress != "192.168.1.1" {
			t.Fatalf("expected IPAddress %q, got %q", "192.168.1.1", stored.IPAddress)
		}
	})

	t.Run("sets expiry roughly 30 days in the future", func(t *testing.T) {
		db := setupTestDB(t)
		userID := uuid.New()

		before := time.Now().Add(sessionDuration)
		_, err := CreateSession(context.Background(), db, userID, "agent", "ip")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		after := time.Now().Add(sessionDuration)

		var stored domain.Session
		if err := db.Where("user_id = ?", userID).First(&stored).Error; err != nil {
			t.Fatalf("expected session to be persisted: %v", err)
		}

		if stored.ExpiresAt.Before(before.Add(-time.Second)) || stored.ExpiresAt.After(after.Add(time.Second)) {
			t.Fatalf("expected ExpiresAt around %v, got %v", before, stored.ExpiresAt)
		}
	})

	t.Run("creating multiple sessions for the same user produces distinct tokens", func(t *testing.T) {
		db := setupTestDB(t)
		userID := uuid.New()

		token1, err := CreateSession(context.Background(), db, userID, "agent-1", "ip-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		token2, err := CreateSession(context.Background(), db, userID, "agent-2", "ip-2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if token1 == token2 {
			t.Fatal("expected distinct tokens for separate sessions")
		}

		var count int64
		db.Model(&domain.Session{}).Where("user_id = ?", userID).Count(&count)
		if count != 2 {
			t.Fatalf("expected 2 sessions stored, got %d", count)
		}
	})
}

func TestValidateSession(t *testing.T) {
	t.Run("validates a freshly created session", func(t *testing.T) {
		db := setupTestDB(t)
		userID := uuid.New()

		rawToken, err := CreateSession(context.Background(), db, userID, "agent", "ip")
		if err != nil {
			t.Fatalf("unexpected error creating session: %v", err)
		}

		session, err := ValidateSession(context.Background(), db, rawToken)
		if err != nil {
			t.Fatalf("expected valid session, got error: %v", err)
		}
		if session.UserID != userID {
			t.Fatalf("expected UserID %v, got %v", userID, session.UserID)
		}
	})

	t.Run("rejects an unknown token", func(t *testing.T) {
		db := setupTestDB(t)

		_, err := ValidateSession(context.Background(), db, "this-token-was-never-issued")
		if err == nil {
			t.Fatal("expected error for unknown token, got nil")
		}
	})

	t.Run("rejects an expired session", func(t *testing.T) {
		db := setupTestDB(t)
		userID := uuid.New()

		rawToken, err := GenerateSessionToken()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expired := domain.Session{
			UserID:    userID,
			TokenHash: HashToken(rawToken),
			UserAgent: "agent",
			IPAddress: "ip",
			ExpiresAt: time.Now().Add(-time.Hour), // already expired
		}
		if err := db.Create(&expired).Error; err != nil {
			t.Fatalf("failed to seed expired session: %v", err)
		}

		_, err = ValidateSession(context.Background(), db, rawToken)
		if err == nil {
			t.Fatal("expected error for expired session, got nil")
		}
	})

	t.Run("rejects a revoked session", func(t *testing.T) {
		db := setupTestDB(t)
		userID := uuid.New()

		rawToken, err := CreateSession(context.Background(), db, userID, "agent", "ip")
		if err != nil {
			t.Fatalf("unexpected error creating session: %v", err)
		}

		if err := RevokeSession(context.Background(), db, rawToken); err != nil {
			t.Fatalf("unexpected error revoking session: %v", err)
		}

		_, err = ValidateSession(context.Background(), db, rawToken)
		if err == nil {
			t.Fatal("expected error for revoked session, got nil")
		}
	})
}

func TestRevokeSession(t *testing.T) {
	t.Run("marks an existing session as revoked", func(t *testing.T) {
		db := setupTestDB(t)
		userID := uuid.New()

		rawToken, err := CreateSession(context.Background(), db, userID, "agent", "ip")
		if err != nil {
			t.Fatalf("unexpected error creating session: %v", err)
		}

		if err := RevokeSession(context.Background(), db, rawToken); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var stored domain.Session
		if err := db.Where("token_hash = ?", HashToken(rawToken)).First(&stored).Error; err != nil {
			t.Fatalf("expected session to still exist: %v", err)
		}
		if !stored.Revoked {
			t.Fatal("expected session to be marked revoked")
		}
	})

	t.Run("revoking an unknown token is a no-op, not an error", func(t *testing.T) {
		db := setupTestDB(t)

		// GORM's Update on a WHERE clause matching zero rows does not
		// return gorm.ErrRecordNotFound; it just affects zero rows.
		err := RevokeSession(context.Background(), db, "never-issued-token")
		if err != nil {
			t.Fatalf("expected no error revoking unknown token, got %v", err)
		}
	})

	t.Run("revoking twice is idempotent", func(t *testing.T) {
		db := setupTestDB(t)
		userID := uuid.New()

		rawToken, err := CreateSession(context.Background(), db, userID, "agent", "ip")
		if err != nil {
			t.Fatalf("unexpected error creating session: %v", err)
		}

		if err := RevokeSession(context.Background(), db, rawToken); err != nil {
			t.Fatalf("unexpected error on first revoke: %v", err)
		}
		if err := RevokeSession(context.Background(), db, rawToken); err != nil {
			t.Fatalf("unexpected error on second revoke: %v", err)
		}
	})
}
