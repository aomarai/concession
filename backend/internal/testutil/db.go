// Package testutil holds helpers shared by tests across packages.
package testutil

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ErrInjected is the error returned by queries made to fail with FailOn.
var ErrInjected = errors.New("injected test failure")

// NewDB opens an isolated in-memory SQLite database named after the test and
// migrates the given models. Foreign-key constraints are not created so tests
// can insert rows without building full object graphs.
func NewDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	// The sequence number keeps databases apart when one test calls NewDB more
	// than once, or is repeated with -count=N.
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"), dbSeq.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// NewFileDB is NewDB backed by a temporary WAL-mode file instead of shared
// in-memory SQLite, which fails concurrent cross-table access with "table is
// locked". Use it for tests that run goroutines against the database.
func NewFileDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db") + "?_busy_timeout=10000&_journal_mode=WAL&_txlock=immediate"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatalf("open sqlite file: %v", err)
	}
	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

var (
	callbackSeq atomic.Int64
	dbSeq       atomic.Int64
)

// FailOn makes every gorm operation of the given kind ("create", "query",
// "update", "delete" or "row") against table return ErrInjected.
func FailOn(t *testing.T, db *gorm.DB, op, table string) {
	t.Helper()
	FailAfter(t, db, op, table, 0)
}

// FailAfter lets the first n matching operations succeed and fails every one
// after that. Use it to reach error paths that follow an earlier successful
// call to the same table.
func FailAfter(t *testing.T, db *gorm.DB, op, table string, n int) {
	t.Helper()
	name := fmt.Sprintf("testutil:fail:%d", callbackSeq.Add(1))
	var seen atomic.Int64
	fn := func(tx *gorm.DB) {
		if tx.Statement.Table == table && seen.Add(1) > int64(n) {
			_ = tx.AddError(ErrInjected)
		}
	}
	var err error
	switch op {
	case "create":
		err = db.Callback().Create().Before("gorm:create").Register(name, fn)
	case "query":
		err = db.Callback().Query().Before("gorm:query").Register(name, fn)
	case "update":
		err = db.Callback().Update().Before("gorm:update").Register(name, fn)
	case "delete":
		err = db.Callback().Delete().Before("gorm:delete").Register(name, fn)
	case "row": // Scan and Row queries do not go through the query callbacks
		err = db.Callback().Row().Before("gorm:row").Register(name, fn)
	default:
		t.Fatalf("unknown op %q", op)
	}
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
}
