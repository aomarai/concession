package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const sessionDuration = 30 * 24 * time.Hour // 30 days

// GenerateSessionToken creates a random, URL-safe token to hand to the client.
// Goes into the cookie. Do not store raw.
func GenerateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		slog.Error("Unable to generate session token", "error", err)
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken hashes a raw session token for storage/lookup.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func CreateSession(ctx context.Context, db *gorm.DB, userID uuid.UUID, userAgent, ipAddress string) (rawToken string, err error) {
	rawToken, err = GenerateSessionToken()
	if err != nil {
		return "", err
	}

	session := domain.Session{
		UserID:    userID,
		TokenHash: HashToken(rawToken),
		UserAgent: userAgent,
		IPAddress: ipAddress,
		ExpiresAt: time.Now().Add(sessionDuration),
	}

	if err := db.WithContext(ctx).Create(&session).Error; err != nil {
		return "", err
	}

	return rawToken, nil
}

func ValidateSession(ctx context.Context, db *gorm.DB, rawToken string) (*domain.Session, error) {
	var session domain.Session
	hash := HashToken(rawToken)

	err := db.WithContext(ctx).Where("token_hash = ? AND revoked = ? AND expires_at > ?", hash, false, time.Now()).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func RevokeSession(ctx context.Context, db *gorm.DB, rawToken string) error {
	hash := HashToken(rawToken)
	return db.WithContext(ctx).
		Model(&domain.Session{}).
		Where("token_hash = ?", hash).
		Update("revoked", true).Error
}
