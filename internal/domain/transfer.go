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
	StatusSentForClearing TransferStatus = "SENT_FOR_CLEARING"
	StatusSettled         TransferStatus = "SETTLED"
)

type TransferOperation string

const (
	NewTransferEvent        TransferOperation = "BANK_ADAPTER_NEW_TRANSFER_EVENT"
	TransferSettledEvent    TransferOperation = "TRANSFER_SETTLED_NOTIFY_EVENT"
	LedgerReserveFundsEvent TransferOperation = "LEDGER_RESERVE_FUNDS_EVENT"
	LedgerSettleFundsEvent  TransferOperation = "LEDGER_SETTLE_FUNDS_EVENT"
)

type Transfer struct {
	Id     uuid.UUID
	Status TransferStatus

	PartnerBank *Bank

	MerchantId      uuid.UUID
	Iban            string
	Payee           string
	PrincipalAmount int64 // Sent over SEPA
	FeeAmount       int64
	TotalDebit      int64 // PrincipalAmount +FeeAmount
	Currency        string
	VoPOverride     bool

	Version int
}

func (t *Transfer) DeriveIdempotencyKey(operation TransferOperation) string {
	idOp := fmt.Sprintf("transfer_op_%s_%s", t.Id.String(), operation)
	//sum := sha256.Sum256([]byte(idOp))
	//return hex.EncodeToString(sum[:])
	return idOp
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

	if newStatus == StatusSentForClearing && t.Status != StatusInitialized {
		return fmt.Errorf("we can't transition transfer from status %s to %s", t.Status, newStatus)
	}
	if newStatus == StatusSettled && t.Status != StatusSentForClearing {
		return fmt.Errorf("we can't transition transfer from status %s to %s", t.Status, newStatus)
	}
	t.Status = newStatus
	return nil
}

func (t *Transfer) SetPartnerBank(bank Bank) error {
	if t.PartnerBank != nil {
		return fmt.Errorf("partner bank is already set for this transfer")
	}
	if t.Status != StatusCreated && t.Status != StatusVoPNoMatch {
		return fmt.Errorf("partner bank cannot be set from status %s", t.Status)
	}
	t.PartnerBank = &bank
	return nil
}

type CreateTransferParams struct {
	MerchantId      uuid.UUID
	Iban            string
	Payee           string
	PrincipalAmount int64
	Currency        string
	ReferenceText   string
}

type TransferRepository interface {
	Create(ctx context.Context, params CreateTransferParams, idempotencyKey string) (*Transfer, error)
	UpdateStatus(ctx context.Context, t *Transfer) error
	UpdateStatusAndPartnerBank(ctx context.Context, t *Transfer) error
	UpdateStatusAndVoPResponse(ctx context.Context, t *Transfer, vopResponseBody string) error
	UpdateStatusAndOutbox(ctx context.Context, t *Transfer, event *OutboxEvent) error
	GetById(ctx context.Context, id uuid.UUID) (*Transfer, error)
	GetCachedResponse(ctx context.Context, idemKey string, merchantId uuid.UUID) (int, string, error)
	UpdateCachedResponse(ctx context.Context, idemKey string, merchantId uuid.UUID, transferId uuid.UUID, respCode int, respBody string) error
	SelectForReconciliation(ctx context.Context) ([]*Transfer, error)
}

type LedgerClient interface {
	ReserveFunds(ctx context.Context, t *Transfer) error
	SettleFunds(ctx context.Context, t *Transfer) error
}

type Bank struct {
	Id   uuid.UUID
	Code string
}

type Router interface {
	GetBank(ctx context.Context, transfer *Transfer) (Bank, error)
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

type FeeCalculator interface {
	CalculateFee(ctx context.Context, merchantId uuid.UUID, Iban string, principalAmount int64, currency string) (int64, error)
}
