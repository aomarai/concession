package userref

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

func TestResolve(t *testing.T) {
	db := testutil.NewDB(t, &domain.User{})
	ann := domain.User{Username: "ann", Email: "ann@example.com", DisplayName: "Ann"}
	if err := db.Create(&ann).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	for name, ref := range map[string]Ref{
		"username": {Identifier: "ann"},
		"e-mail":   {Identifier: "ANN@example.com"},
		"id":       {ID: ann.ID},
	} {
		if u, err := Resolve(ctx, db, ref); err != nil || u.ID != ann.ID {
			t.Errorf("%s: %v, %v", name, u.ID, err)
		}
	}

	cases := map[string]struct {
		ref  Ref
		want error
		msg  string
	}{
		"empty":        {Ref{}, svcerr.ErrInvalid, "user is required"},
		"blank":        {Ref{Identifier: "   "}, svcerr.ErrInvalid, "user is required"},
		"both":         {Ref{Identifier: "ann", ID: ann.ID}, svcerr.ErrInvalid, "either"},
		"unknown name": {Ref{Identifier: "nobody"}, svcerr.ErrNotFound, "No user found"},
		"unknown id":   {Ref{ID: uuid.New()}, svcerr.ErrNotFound, "No user found"},
	}
	for name, tc := range cases {
		_, err := Resolve(ctx, db, tc.ref)
		if !errors.Is(err, tc.want) || !strings.Contains(svcerr.MessageOr(err, ""), tc.msg) {
			t.Errorf("%s: %v", name, err)
		}
	}

	testutil.FailOn(t, db, "query", "users")
	for _, ref := range []Ref{{Identifier: "ann"}, {ID: ann.ID}} {
		if _, err := Resolve(ctx, db, ref); !errors.Is(err, testutil.ErrInjected) {
			t.Errorf("%+v: %v", ref, err)
		}
	}
}
