package domain

import (
	"context"

	"github.com/google/uuid"
)

type TransferStatus string

const (
	StatusCreated         TransferStatus = "CREATED"
	StatusAntifraudFailed TransferStatus = "ANTIFRAUD_FAILED"
	StatusVoPNoMatch      TransferStatus = "VOP_NO_MATCH"
	StatusRoutingFailed   TransferStatus = "ROUTING_FAILED"
	StatusInitialized     TransferStatus = "INITIALIZED"
	StatusFundsHolded     TransferStatus = "FUNDS_HOLDED"
	StatusCleared         TransferStatus = "CLEARED"
	StatusSettled         TransferStatus = "SETTLED"
)

type Transfer struct {
	Id     uuid.UUID
	Status TransferStatus

	PartnerBank *uuid.UUID

	MerchantId uuid.UUID
	Iban       string
	Payee      string
	Amount     int64
	Currency   string

	Version int
}

type CreateTransferParams struct {
	MerchantId    uuid.UUID
	Iban          string
	Payee         string
	Amount        int64
	Currency      string
	ReferenceText string
}

type TransferRepository interface {
	Create(ctx context.Context, params CreateTransferParams, idempotencyKey string) (*Transfer, error)
	UpdateStatus(ctx context.Context, t *Transfer, newStatus TransferStatus) error
	SetPartnerBank(ctx context.Context, t *Transfer, bankId uuid.UUID, newStatus TransferStatus) error
	UpdateStatusAndOutbox(ctx context.Context, t *Transfer, newStatus TransferStatus, event *OutboxEvent) error
}

type LedgerClient interface {
	ReserveFunds(ctx context.Context, t *Transfer, idempotencyKey string) error
	SettleFunds(ctx context.Context, t *Transfer, idempotencyKey string) error
}

type Router interface {
	GetBank(ctx context.Context, transfer *Transfer) (uuid.UUID, error)
}

type VoPCode string

const (
	VoPMatch       VoPCode = "Match"
	VoPNoMatch     VoPCode = "No_Match"
	VoPCloseMatch  VoPCode = "Close_Match"
	VoPNotPossible VoPCode = "Not_Possible"
)

type VoPChecker interface {
	Check(ctx context.Context, iban string, payee string) (VoPCode, string, error)
}

type AntifraudChecker interface {
	Check(ctx context.Context, transfer *Transfer) (bool, error)
}
