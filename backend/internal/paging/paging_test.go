package paging

import (
	"errors"
	"testing"

	"github.com/aomarai/concession/internal/svcerr"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		page, perPage         int
		wantPage, wantPerPage int
		wantErr               bool
	}{
		{0, 0, 1, DefaultPerPage, false},
		{3, 50, 3, 50, false},
		{1, 5000, 1, MaxPerPage, false},
		{-1, 10, 0, 0, true},
		{1, -1, 0, 0, true},
		{MaxPage, 1, MaxPage, 1, false},
		{MaxPage + 1, 1, 0, 0, true},
		{int(^uint(0) >> 1), 100, 0, 0, true},
	}
	for _, tc := range cases {
		page, perPage, err := Normalize(tc.page, tc.perPage)
		if tc.wantErr {
			if !errors.Is(err, svcerr.ErrInvalid) {
				t.Errorf("%+v: expected a validation error, got %v", tc, err)
			}
			continue
		}
		if err != nil || page != tc.wantPage || perPage != tc.wantPerPage {
			t.Errorf("%+v: got %d, %d, %v", tc, page, perPage, err)
		}
	}
}

func TestOffset(t *testing.T) {
	if Offset(1, 20) != 0 || Offset(3, 20) != 40 {
		t.Error("unexpected offsets")
	}
}
