// Package model contains the shared domain types used by both the Gateway Daemon
// and the Central Server.
package model

import "errors"

var (
	// ErrNotFound is returned when a requested resource does not exist in the store.
	ErrNotFound = errors.New("not found")

	// ErrUnauthorized is returned when the request lacks valid credentials or the
	// provided credentials do not grant access to the requested resource.
	ErrUnauthorized = errors.New("unauthorized")
)
