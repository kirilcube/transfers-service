package domain

import "errors"

var (
	// ErrConcurrentModification is returned when the version check fails (OCC)
	ErrConcurrentModification = errors.New("transfer was modified by another process")

	// ErrIdempotencyConflict is returned when a duplicate request is processing
	ErrIdempotencyConflict = errors.New("request is already processing")

	// ErrVoPOverrideRequired is returned when we can't proceed unless user confirms overriding VoP
	ErrVoPOverrideRequired = errors.New("vop is not matching, override required")

	// ErrAntifraudFailed is returned when antifraud check was failed
	ErrAntifraudFailed = errors.New("transfer's antifraud check was failed")

	// ErrInsufficientFunds is returned by the Ledger
	//ErrInsufficientFunds      = errors.New("merchant has insufficient funds")
)
