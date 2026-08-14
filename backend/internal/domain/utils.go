package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type BaseUUID struct {
	ID        uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`
}

// BeforeCreate runs for any struct embedding BaseUUID
func (base *BaseUUID) BeforeCreate(_ *gorm.DB) (err error) {
	if base.ID == uuid.Nil {
		base.ID = uuid.New()
	}
	return nil
}

// uniqueSQLiteDSN returns a SQLite in-memory DSN scoped to the current test
// (or subtest) by name.
//
// All of this package's setupXTestDB helpers use "cache=shared" so that
// GORM's connection pool (which may open more than one connection) sees
// the same in-memory database within a single test. But cache=shared with
// an unnamed/empty database name means EVERY connection opened with that
// DSN across the whole test binary shares the same underlying SQLite
// database — not just connections within one test. Without a unique name
// per test, data from one test (or subtest) silently leaks into another
// that happens to run in the same process. That's what caused
// TestHandleGoogleCallback's session counts to include leftover sessions
// from unrelated tests.
func uniqueSQLiteDSN(t *testing.T) string {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	return "file:" + name + "?mode=memory&cache=shared"
}
