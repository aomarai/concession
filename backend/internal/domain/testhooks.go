package domain

import "crypto/rand"

// SetRandReadForTest replaces the entropy source used for share tokens so
// tests in other packages can simulate failures. Call ResetRandReadForTest
// when done.
func SetRandReadForTest(f func([]byte) (int, error)) { randRead = f }

// ResetRandReadForTest restores the real entropy source.
func ResetRandReadForTest() { randRead = rand.Read }
