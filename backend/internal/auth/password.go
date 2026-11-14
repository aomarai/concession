package auth

import (
	"fmt"
	"unicode/utf8"

	"github.com/aomarai/concession/internal/svcerr"
	"golang.org/x/crypto/bcrypt"
)

const (
	// MinPasswordLength is the shortest password accepted, counted in
	// characters so a non-ASCII passphrase is judged the way a person reads it.
	MinPasswordLength = 12
	// MaxPasswordLength is bcrypt's input limit: it silently ignores anything
	// past 72 bytes, so a longer password must be refused rather than truncated.
	MaxPasswordLength = 72
)

// bcryptCost is a variable so tests can drop it to bcrypt.MinCost; production
// always hashes at 12.
var bcryptCost = 12

// bcryptGenerate is a variable so tests can inject a hashing failure.
var bcryptGenerate = bcrypt.GenerateFromPassword

// dummyHash is compared against when there is no real hash to compare (an
// unknown identifier, or an account with no local password), so a failed sign-in
// costs the same time whether or not the account exists. It is the cost-12 hash
// of "concession-dummy-password".
const dummyHash = "$2a$12$AEUz74Mc6H2/WdQGcNIrxukPz.M72xCasMm2S/WwS4Wl4q4.uGziS"

// ValidatePassword enforces the password policy. The error is a validation
// error, so its message is safe to show the person signing up.
func ValidatePassword(plain string) error {
	switch {
	case utf8.RuneCountInString(plain) < MinPasswordLength:
		return svcerr.Invalid(fmt.Sprintf("password must be at least %d characters", MinPasswordLength))
	case len(plain) > MaxPasswordLength:
		return svcerr.Invalid(fmt.Sprintf("password must be at most %d bytes", MaxPasswordLength))
	}
	return nil
}

// HashPassword validates and hashes a new password for storage.
func HashPassword(plain string) (string, error) {
	if err := ValidatePassword(plain); err != nil {
		return "", err
	}
	hash, err := bcryptGenerate([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether plain matches the stored hash. bcrypt's compare
// is constant-time; a malformed or empty hash simply does not match.
func CheckPassword(hash, plain string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// burnPasswordCheck spends one bcrypt comparison so an authentication failure
// for a missing account takes as long as one for a wrong password.
func burnPasswordCheck(plain string) {
	_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(plain))
}
