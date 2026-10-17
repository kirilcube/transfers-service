package domain_test

import (
	"testing"
	"transfers/internal/domain"

	"github.com/google/uuid"
)

//const (
//	StatusCreated         TransferStatus = "CREATED"
//	StatusAntifraudFailed TransferStatus = "ANTIFRAUD_FAILED"
//	StatusVoPNoMatch      TransferStatus = "VOP_NO_MATCH"
//	StatusRoutingFailed   TransferStatus = "ROUTING_FAILED"
//	StatusInitialized     TransferStatus = "INITIALIZED"
//	StatusFundsHolded     TransferStatus = "FUNDS_HOLDED"
//	StatusCleared         TransferStatus = "CLEARED"
//	StatusSettled         TransferStatus = "SETTLED"
//)

func TestTransfer_TransitionTo(t *testing.T) {
	tests := []struct {
		name          string
		currentStatus domain.TransferStatus
		targetStatus  domain.TransferStatus
		expectError   bool
	}{
		// happy path
		{"allow CREATED to ANTIFRAUD_FAILED", domain.StatusCreated, domain.StatusAntifraudFailed, false},
		{"allow CREATED to VOP_NO_MATCH", domain.StatusCreated, domain.StatusVoPNoMatch, false},
		{"allow CREATED to ROUTING_FAILED", domain.StatusCreated, domain.StatusRoutingFailed, false},
		{"allow CREATED to INITIALIZED", domain.StatusCreated, domain.StatusInitialized, false},
		{"allow VOP_NO_MATCH to INITIALIZED", domain.StatusVoPNoMatch, domain.StatusInitialized, false},
		{"allow INITIALIZED to SENT_FOR_CLEARING", domain.StatusInitialized, domain.StatusSentForClearing, false},
		{"allow SENT_FOR_CLEARING to SETTLED", domain.StatusSentForClearing, domain.StatusSettled, false},

		// invalid transitions
		{"restrict SETTLED to SENT_FOR_CLEARING", domain.StatusSettled, domain.StatusSentForClearing, true},
		{"restrict ANTIFRAUD_FAILED to INITIALIZED", domain.StatusAntifraudFailed, domain.StatusInitialized, true},
		{"restrict ROUTING_FAILED to INITIALIZED", domain.StatusRoutingFailed, domain.StatusInitialized, true},
		{"restrict VOP_NO_MATCH to CREATED", domain.StatusVoPNoMatch, domain.StatusCreated, true},
		{"restrict INITIALIZED to SETTLED", domain.StatusInitialized, domain.StatusSettled, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transfer := &domain.Transfer{
				Status: test.currentStatus,
			}
			err := transfer.TransitionTo(test.targetStatus)
			if err != nil && !test.expectError {
				t.Errorf("didn't expect error, but received one: %v", err)
			} else if err == nil && test.expectError {
				t.Errorf("expected error")
			}
			if err == nil && !test.expectError && transfer.Status != test.targetStatus {
				t.Errorf("transfer.Status(%s) is not targetStatus", transfer.Status)
			}
		})
	}
}

func TestTransfer_SetPartnerBank(t *testing.T) {
	bank1 := domain.Bank{
		Id:   uuid.New(),
		Code: "Bank1",
	}
	bank2 := domain.Bank{
		Id:   uuid.New(),
		Code: "Bank2",
	}
	tests := []struct {
		name          string
		targetBank    domain.Bank
		currentBank   *domain.Bank
		currentStatus domain.TransferStatus
		expectError   bool
	}{
		// happy path
		{"setting bank id status CREATED", bank1, nil, domain.StatusCreated, false},
		{"setting bank id status VoPNoMatch", bank1, nil, domain.StatusVoPNoMatch, false},

		// error path
		{"setting bank id when already set", bank1, &bank2, domain.StatusCreated, true},
		{"setting bank id when status INITIALIZED", bank1, nil, domain.StatusInitialized, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transfer := &domain.Transfer{
				PartnerBank: test.currentBank,
				Status:      test.currentStatus,
			}
			err := transfer.SetPartnerBank(test.targetBank)
			if err != nil && !test.expectError {
				t.Errorf("didn't expect error, but received one: %v", err)
			} else if err == nil && test.expectError {
				t.Errorf("expected error")
			}
			if err == nil && !test.expectError && (transfer.PartnerBank == nil || transfer.PartnerBank.Id != test.targetBank.Id) {
				t.Errorf("transfer.PartnerBank is not targetBankId")
			}
		})
	}
}

func TestTransfer_DeriveIdempotencyKey(t *testing.T) {
	t.Run("Two derived keys should be the same", func(t *testing.T) {
		transfer := &domain.Transfer{
			Status: domain.StatusInitialized,
			Id:     uuid.New(),
		}
		var operation domain.TransferOperation = "TEST"
		idemKey1 := transfer.DeriveIdempotencyKey(operation)
		idemKey2 := transfer.DeriveIdempotencyKey(operation)
		if idemKey1 != idemKey2 {
			t.Errorf("not the same: idemKey1(%s) != idemKey2(%s)", idemKey1, idemKey2)
		}
	})
}
