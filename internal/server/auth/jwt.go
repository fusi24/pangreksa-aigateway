// Package auth provides JWT signing/validation and password hashing utilities
// for the Pangreksa AI Gateway Console authentication layer.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// jwtHeader is the fixed base64url-encoded JWT header for HS256.
// Precomputed to avoid redundant encoding on every Sign call.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// JWTService signs and validates HS256 JSON Web Tokens.
//
// The secret is typically sourced from the NEXTAUTH_SECRET or JWT_SECRET
// environment variable and must be at least 32 bytes for adequate security.
//
// Thread-safety: safe for concurrent use; all state is read-only after construction.
type JWTService struct {
	// secret is the HMAC-SHA256 signing key.
	secret []byte
}

// Claims holds the console JWT payload.
// All fields are included in the signed token and must not be modified after signing.
type Claims struct {
	// UserID is the UUID of the authenticated user.
	UserID string `json:"user_id"`
	// OrgID is the UUID of the user's organization.
	OrgID string `json:"org_id"`
	// Email is the user's email address.
	Email string `json:"email"`
	// Perms is the list of permission tokens granted to the user.
	Perms []string `json:"perms"`
	// Exp is the Unix timestamp (seconds) when the token expires.
	Exp int64 `json:"exp"`
	// Iat is the Unix timestamp (seconds) when the token was issued.
	Iat int64 `json:"iat"`
}

// NewJWTService constructs a JWTService using the provided secret string.
// The secret is stored as raw bytes; an empty secret is accepted but insecure.
func NewJWTService(secret string) *JWTService {
	return &JWTService{secret: []byte(secret)}
}

// Sign generates a signed HS256 JWT string for the given claims.
// The Iat field is set to now and Exp is set to now + 15 minutes if not already set.
// Returns the compact serialisation "header.payload.signature".
func (s *JWTService) Sign(claims Claims) (string, error) {
	now := time.Now().UTC().Unix()
	if claims.Iat == 0 {
		claims.Iat = now
	}
	if claims.Exp == 0 {
		claims.Exp = now + 900 // 15 minutes
	}

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("JWTService.Sign: marshal claims: %w", err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signingInput := jwtHeader + "." + encodedPayload

	sig, err := s.sign(signingInput)
	if err != nil {
		return "", fmt.Errorf("JWTService.Sign: compute signature: %w", err)
	}

	return signingInput + "." + sig, nil
}

// Validate parses and validates a compact JWT string.
//
// It verifies the HMAC-SHA256 signature and checks the exp claim.
// Returns a pointer to the decoded Claims on success, or an error when
// the token is malformed, the signature is invalid, or the token has expired.
func (s *JWTService) Validate(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("JWTService.Validate: malformed token: expected 3 parts, got %d", len(parts))
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig, err := s.sign(signingInput)
	if err != nil {
		return nil, fmt.Errorf("JWTService.Validate: compute expected signature: %w", err)
	}

	// constant-time comparison prevents timing attacks on the signature.
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, errors.New("JWTService.Validate: invalid signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("JWTService.Validate: decode payload: %w", err)
	}

	var claims Claims
	if err = json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("JWTService.Validate: unmarshal claims: %w", err)
	}

	if time.Now().UTC().Unix() > claims.Exp {
		return nil, errors.New("JWTService.Validate: token has expired")
	}

	return &claims, nil
}

// GenerateRefreshToken returns a cryptographically random 64-byte hex string
// (128 hex characters) suitable for use as an opaque refresh token.
func GenerateRefreshToken() (string, error) {
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("GenerateRefreshToken: read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// sign computes the base64url-encoded HMAC-SHA256 signature of input using the
// service's secret key.
func (s *JWTService) sign(input string) (string, error) {
	mac := hmac.New(sha256.New, s.secret)
	if _, err := mac.Write([]byte(input)); err != nil {
		return "", fmt.Errorf("sign: write to hmac: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
