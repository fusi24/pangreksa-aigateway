// Package crypto provides AES-256-GCM authenticated encryption helpers used to
// protect LLM provider API keys at rest and in the GatewayConfig wire format.
//
// Key management:
//   - The encryption key is a 32-byte (256-bit) value loaded from the ENCRYPT_KEY
//     environment variable, which must contain exactly 64 hex characters.
//   - Keys are never logged, never included in error messages, and never written
//     to any persistent store in plaintext.
//
// Ciphertext format (returned by Encrypt):
//
//	[ 12-byte random nonce ][ GCM ciphertext + 16-byte tag ]
//
// This layout allows Decrypt to extract the nonce from the same byte slice without
// any additional framing, making the format self-contained.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

const (
	// keyLen is the required AES-256 key length in bytes.
	keyLen = 32
	// nonceLen is the standard GCM nonce length in bytes.
	nonceLen = 12
)

// Encrypt encrypts plaintext using AES-256-GCM with a random 12-byte nonce.
// The returned ciphertext is: [ nonce (12 bytes) | encrypted data | GCM tag (16 bytes) ].
//
// key must be exactly 32 bytes; use KeyFromHex to derive it from the ENCRYPT_KEY env var.
// plaintext may be empty, in which case only the GCM tag is appended.
//
// Returns a non-nil error if the key length is wrong or random nonce generation fails.
func Encrypt(key []byte, plaintext []byte) ([]byte, error) {
	if len(key) != keyLen {
		return nil, fmt.Errorf("crypto.Encrypt: key must be %d bytes, got %d", keyLen, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto.Encrypt: create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto.Encrypt: create GCM: %w", err)
	}

	// allocate nonce + expected ciphertext in one slice to avoid extra allocations.
	nonce := make([]byte, nonceLen)
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto.Encrypt: generate nonce: %w", err)
	}

	// gcm.Seal appends the ciphertext + tag to nonce, giving us the self-contained format.
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts a ciphertext produced by Encrypt.
// It expects the first 12 bytes to be the nonce, followed by the GCM ciphertext and tag.
//
// key must be exactly 32 bytes; use KeyFromHex to derive it from the ENCRYPT_KEY env var.
//
// Returns ErrCiphertextTooShort if the input is shorter than nonce + tag (28 bytes).
// Returns an error wrapping cipher.ErrAuthFailed if the ciphertext is tampered or the
// wrong key is used.
func Decrypt(key []byte, ciphertext []byte) ([]byte, error) {
	if len(key) != keyLen {
		return nil, fmt.Errorf("crypto.Decrypt: key must be %d bytes, got %d", keyLen, len(key))
	}

	// minimum valid ciphertext: nonce (12) + GCM tag (16) = 28 bytes.
	if len(ciphertext) < nonceLen+16 {
		return nil, fmt.Errorf("crypto.Decrypt: ciphertext too short (%d bytes)", len(ciphertext))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto.Decrypt: create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto.Decrypt: create GCM: %w", err)
	}

	// extract the nonce from the first 12 bytes.
	nonce := ciphertext[:nonceLen]
	data := ciphertext[nonceLen:]

	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto.Decrypt: authentication failed: %w", err)
	}

	return plaintext, nil
}

// KeyFromHex decodes a 64-character hex string into a 32-byte AES-256 key.
// The hexStr argument is typically the value of the ENCRYPT_KEY environment variable.
//
// Returns an error if hexStr is not valid hex or does not decode to exactly 32 bytes.
func KeyFromHex(hexStr string) ([]byte, error) {
	key, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("crypto.KeyFromHex: decode hex: %w", err)
	}
	if len(key) != keyLen {
		return nil, fmt.Errorf("crypto.KeyFromHex: expected %d bytes, got %d (hex string must be %d chars)",
			keyLen, len(key), keyLen*2)
	}
	return key, nil
}
