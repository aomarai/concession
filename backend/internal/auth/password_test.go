package auth

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/svcerr"
	"golang.org/x/crypto/bcrypt"
)

func TestMain(m *testing.M) {
	// Production hashes at cost 12; the suite only cares about behaviour, and a
	// real cost would add a bcrypt round to every test that signs someone in.
	bcryptCost = bcrypt.MinCost
	os.Exit(m.Run())
}

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"empty", "", true},
		{"too short", strings.Repeat("a", MinPasswordLength-1), true},
		{"minimum length", strings.Repeat("a", MinPasswordLength), false},
		{"longer", strings.Repeat("a", 32), false},
		{"exactly the byte limit", strings.Repeat("a", MaxPasswordLength), false},
		{"past the byte limit", strings.Repeat("a", MaxPasswordLength+1), true},
		// Twelve characters of two bytes each is 24 bytes: fine, because the
		// limit counts characters and the byte limit is still under 72.
		{"multibyte within the limits", strings.Repeat("é", MinPasswordLength), false},
		// 37 two-byte characters is 74 bytes, so bcrypt would truncate it.
		{"multibyte past the byte limit", strings.Repeat("é", 37), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.password)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				if !isInvalid(err) {
					t.Errorf("expected a validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// isInvalid reports whether err is a validation error (svcerr.ErrInvalid).
func isInvalid(err error) bool {
	return errors.Is(err, svcerr.ErrInvalid)
}

func TestHashPasswordRoundTrip(t *testing.T) {
	plain := "correct horse battery staple"
	hash, err := HashPassword(plain)
	if err != nil {
		t.Fatal(err)
	}
	if hash == plain || !strings.HasPrefix(hash, "$2") {
		t.Fatalf("expected a bcrypt hash, got %q", hash)
	}
	if !CheckPassword(hash, plain) {
		t.Error("the password should match its own hash")
	}
	if CheckPassword(hash, plain+"!") {
		t.Error("a different password must not match")
	}
	if CheckPassword(hash, "") {
		t.Error("an empty password must not match")
	}
}

func TestHashPasswordRejectsWeakPasswords(t *testing.T) {
	if _, err := HashPassword("short"); !isInvalid(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

func TestCheckPasswordRejectsEmptyAndMalformedHashes(t *testing.T) {
	if CheckPassword("", strings.Repeat("a", MinPasswordLength)) {
		t.Error("an empty hash must never match")
	}
	if CheckPassword("not-a-bcrypt-hash", strings.Repeat("a", MinPasswordLength)) {
		t.Error("a malformed hash must never match")
	}
}

func TestBurnPasswordCheckSpendsWorkWithoutPanicking(t *testing.T) {
	burnPasswordCheck("anything")
}

func TestHashPasswordSurfacesHashingFailures(t *testing.T) {
	original := bcryptGenerate
	bcryptGenerate = func([]byte, int) ([]byte, error) { return nil, errors.New("no entropy") }
	t.Cleanup(func() { bcryptGenerate = original })

	if _, err := HashPassword(strings.Repeat("a", MinPasswordLength)); err == nil {
		t.Fatal("expected the hashing error to be returned")
	}
}
