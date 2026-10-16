package domain

import "errors"

var (
	// ErrConcurrentModification is returned when the version check fails (OCC)
	ErrConcurrentModification = errors.New("transfer was modified by another process")

	// ErrIdempotencyConflict is returned when a duplicate request is processing
	ErrIdempotencyConflict = errors.New("request is already processing")

	// ErrInsufficientFunds is returned by the Ledger
	//ErrInsufficientFunds      = errors.New("merchant has insufficient funds")
)
