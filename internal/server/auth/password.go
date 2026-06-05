package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns the bcrypt hash of plaintext using cost factor 12.
//
// The returned hash string is suitable for direct storage in the database
// and includes the cost, salt, and digest in a single portable string.
func HashPassword(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), 12)
	if err != nil {
		return "", fmt.Errorf("HashPassword: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a plaintext password against a stored bcrypt hash.
//
// Returns nil when the password matches the hash, or a non-nil error
// (bcrypt.ErrMismatchedHashAndPassword) when it does not.
func CheckPassword(hash, plaintext string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)); err != nil {
		return fmt.Errorf("CheckPassword: %w", err)
	}
	return nil
}
