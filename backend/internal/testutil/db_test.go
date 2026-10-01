package testutil

import (
	"errors"
	"testing"
)

type row struct {
	ID   uint
	Name string
}

func TestFailOn(t *testing.T) {
	for _, op := range []string{"create", "query", "update", "delete"} {
		t.Run(op, func(t *testing.T) {
			db := NewDB(t, &row{})
			base := row{Name: "a"}
			if err := db.Create(&base).Error; err != nil {
				t.Fatal(err)
			}
			FailOn(t, db, op, "rows")

			var err error
			switch op {
			case "create":
				err = db.Create(&row{Name: "b"}).Error
			case "query":
				err = db.First(&row{}).Error
			case "update":
				err = db.Model(&base).Update("name", "c").Error
			case "delete":
				err = db.Delete(&base).Error
			}
			if !errors.Is(err, ErrInjected) {
				t.Errorf("expected ErrInjected, got %v", err)
			}
		})
	}
}

func TestFailOnOtherTableUnaffected(t *testing.T) {
	db := NewDB(t, &row{})
	FailOn(t, db, "create", "other_things")
	if err := db.Create(&row{Name: "ok"}).Error; err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
