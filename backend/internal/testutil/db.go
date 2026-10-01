// Package testutil holds helpers shared by tests across packages.
package testutil

import (
	"errors"
	"fmt"
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
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
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

var callbackSeq atomic.Int64

// FailOn makes every gorm operation of the given kind ("create", "query",
// "update" or "delete") against table return ErrInjected.
func FailOn(t *testing.T, db *gorm.DB, op, table string) {
	t.Helper()
	name := fmt.Sprintf("testutil:fail:%d", callbackSeq.Add(1))
	fn := func(tx *gorm.DB) {
		if tx.Statement.Table == table {
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
	default:
		t.Fatalf("unknown op %q", op)
	}
	if err != nil {
		t.Fatalf("register callback: %v", err)
	}
}
