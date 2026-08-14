package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupOAuthSessionTestDB spins up an in-memory SQLite DB for OAuthAccount
// and Session tests. FK constraint creation is disabled during migration
// since both models reference User, and these tests only ever store a bare
// UUID in UserID — never a persisted User row.
func setupOAuthSessionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(uniqueSQLiteDSN(t)), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}

	if err := db.AutoMigrate(&OAuthAccount{}, &Session{}); err != nil {
		t.Fatalf("failed to migrate OAuthAccount/Session: %v", err)
	}

	return db
}

func TestOAuthAccountUniqueProviderUser(t *testing.T) {
	db := setupOAuthSessionTestDB(t)

	first := OAuthAccount{UserID: uuid.New(), Provider: "google", ProviderUserID: "12345"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("unexpected error creating first oauth account: %v", err)
	}

	t.Run("rejects duplicate provider + provider_user_id", func(t *testing.T) {
		dup := OAuthAccount{UserID: uuid.New(), Provider: "google", ProviderUserID: "12345"}
		if err := db.Create(&dup).Error; err == nil {
			t.Fatal("expected error creating duplicate (provider, provider_user_id) pair, got nil")
		}
	})

	t.Run("allows same provider with a different provider_user_id", func(t *testing.T) {
		other := OAuthAccount{UserID: uuid.New(), Provider: "google", ProviderUserID: "67890"}
		if err := db.Create(&other).Error; err != nil {
			t.Fatalf("unexpected error creating account with different provider_user_id: %v", err)
		}
	})

	t.Run("allows a different provider with the same provider_user_id", func(t *testing.T) {
		other := OAuthAccount{UserID: uuid.New(), Provider: "github", ProviderUserID: "12345"}
		if err := db.Create(&other).Error; err != nil {
			t.Fatalf("unexpected error creating account with different provider: %v", err)
		}
	})
}

func TestOAuthAccountJSONExcludesTokens(t *testing.T) {
	acct := OAuthAccount{
		UserID:         uuid.New(),
		Provider:       "google",
		ProviderUserID: "12345",
		AccessToken:    "super-secret-access-token",
		RefreshToken:   "super-secret-refresh-token",
	}

	data, err := json.Marshal(acct)
	if err != nil {
		t.Fatalf("unexpected error marshaling OAuthAccount: %v", err)
	}

	out := string(data)
	if strings.Contains(out, "super-secret-access-token") {
		t.Error("AccessToken value leaked into JSON output despite json:\"-\" tag")
	}
	if strings.Contains(out, "super-secret-refresh-token") {
		t.Error("RefreshToken value leaked into JSON output despite json:\"-\" tag")
	}
	if strings.Contains(out, "AccessToken") || strings.Contains(out, "access_token") {
		t.Error("AccessToken field name leaked into JSON output")
	}
	if strings.Contains(out, "RefreshToken") || strings.Contains(out, "refresh_token") {
		t.Error("RefreshToken field name leaked into JSON output")
	}
}

func TestSessionJSONExcludesTokenHash(t *testing.T) {
	session := Session{
		UserID:    uuid.New(),
		TokenHash: "super-secret-token-hash",
		UserAgent: "test-agent",
		IPAddress: "127.0.0.1",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("unexpected error marshaling Session: %v", err)
	}

	out := string(data)
	if strings.Contains(out, "super-secret-token-hash") {
		t.Error("TokenHash value leaked into JSON output despite json:\"-\" tag")
	}
	if strings.Contains(out, "TokenHash") || strings.Contains(out, "token_hash") {
		t.Error("TokenHash field name leaked into JSON output")
	}

	// Fields that are NOT tagged json:"-" should still be present.
	if !strings.Contains(out, "user_agent") {
		t.Error("expected user_agent to be present in JSON output")
	}
	if !strings.Contains(out, "ip_address") {
		t.Error("expected ip_address to be present in JSON output")
	}
}

func TestSessionRevokedDefaultsFalse(t *testing.T) {
	db := setupOAuthSessionTestDB(t)

	session := Session{
		UserID:    uuid.New(),
		TokenHash: "some-hash",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("unexpected error creating session: %v", err)
	}

	var fetched Session
	if err := db.First(&fetched, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching session: %v", err)
	}
	if fetched.Revoked {
		t.Error("expected Revoked to default to false")
	}
}

func TestOAuthAccountAndSessionBaseUUIDPopulatesID(t *testing.T) {
	db := setupOAuthSessionTestDB(t)

	acct := OAuthAccount{UserID: uuid.New(), Provider: "google", ProviderUserID: "id-check"}
	if err := db.Create(&acct).Error; err != nil {
		t.Fatalf("unexpected error creating oauth account: %v", err)
	}
	if acct.ID == uuid.Nil {
		t.Error("expected OAuthAccount.ID to be populated by BaseUUID's BeforeCreate hook")
	}

	session := Session{UserID: uuid.New(), TokenHash: "id-check-hash", ExpiresAt: time.Now().Add(time.Hour)}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("unexpected error creating session: %v", err)
	}
	if session.ID == uuid.Nil {
		t.Error("expected Session.ID to be populated by BaseUUID's BeforeCreate hook")
	}
}
