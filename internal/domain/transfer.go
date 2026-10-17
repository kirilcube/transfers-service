package domain

import (
	"context"
	"fmt"

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

func (t *Transfer) TransitionTo(newStatus TransferStatus) error {
	if newStatus == StatusCreated {
		return fmt.Errorf("we can't transition transfer to created status")
	}
	if t.Status == StatusAntifraudFailed ||
		t.Status == StatusRoutingFailed ||
		t.Status == StatusSettled {
		return fmt.Errorf("we can't change status of transfer from %s", t.Status)
	}

	if (newStatus == StatusAntifraudFailed ||
		newStatus == StatusVoPNoMatch ||
		newStatus == StatusRoutingFailed) && t.Status != StatusCreated {
		return fmt.Errorf("we can't transition transfer from status %s to %s", t.Status, newStatus)
	}

	if newStatus == StatusInitialized &&
		(t.Status != StatusCreated && t.Status != StatusVoPNoMatch) {
		return fmt.Errorf("we can't transition transfer from status %s to %s", t.Status, newStatus)
	}

	if newStatus == StatusFundsHolded && t.Status != StatusInitialized {
		return fmt.Errorf("we can't transition transfer from status %s to %s", t.Status, newStatus)
	}
	if newStatus == StatusCleared && t.Status != StatusFundsHolded {
		return fmt.Errorf("we can't transition transfer from status %s to %s", t.Status, newStatus)
	}
	if newStatus == StatusSettled && t.Status != StatusCleared {
		return fmt.Errorf("we can't transition transfer from status %s to %s", t.Status, newStatus)
	}
	t.Status = newStatus
	return nil
}

func (t *Transfer) SetPartnerBank(id uuid.UUID) error {
	if t.PartnerBank != nil {
		return fmt.Errorf("partner bank is already set for this transfer")
	}
	if t.Status != StatusCreated && t.Status != StatusVoPNoMatch {
		return fmt.Errorf("partner bank cannot be set from status %s", t.Status)
	}
	t.PartnerBank = &id
	return nil
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
	UpdateStatus(ctx context.Context, t *Transfer) error
	UpdateStatusAndPartnerBank(ctx context.Context, t *Transfer) error
	UpdateStatusAndOutbox(ctx context.Context, t *Transfer, event *OutboxEvent) error
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
