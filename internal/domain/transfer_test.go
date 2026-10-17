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
		{"allow INITIALIZED to FUNDS_HOLDED", domain.StatusInitialized, domain.StatusFundsHolded, false},
		{"allow FUNDS_HOLDED to CLEARED", domain.StatusFundsHolded, domain.StatusCleared, false},
		{"allow CLEARED to SETTLED", domain.StatusCleared, domain.StatusSettled, false},

		// invalid transitions
		{"restrict SETTLED to CLEARED", domain.StatusSettled, domain.StatusCleared, true},
		{"restrict ANTIFRAUD_FAILED to INITIALIZED", domain.StatusAntifraudFailed, domain.StatusInitialized, true},
		{"restrict ROUTING_FAILED to INITIALIZED", domain.StatusRoutingFailed, domain.StatusInitialized, true},
		{"restrict VOP_NO_MATCH to CREATED", domain.StatusVoPNoMatch, domain.StatusCreated, true},
		{"restrict INITIALIZED to CLEARED", domain.StatusInitialized, domain.StatusCleared, true},
		{"restrict INITIALIZED to SETTLED", domain.StatusInitialized, domain.StatusSettled, true},
		{"restrict FUNDS_HOLDED to SETTLED", domain.StatusFundsHolded, domain.StatusSettled, true},
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
	id := uuid.New()
	id2 := uuid.New()
	tests := []struct {
		name          string
		targetBankId  uuid.UUID
		currentBankId *uuid.UUID
		currentStatus domain.TransferStatus
		expectError   bool
	}{
		// happy path
		{"setting bank id status CREATED", id, nil, domain.StatusCreated, false},
		{"setting bank id status VoPNoMatch", id, nil, domain.StatusVoPNoMatch, false},

		// error path
		{"setting bank id when already set", id, &id2, domain.StatusCreated, true},
		{"setting bank id when status INITIALIZED", id, nil, domain.StatusInitialized, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transfer := &domain.Transfer{
				PartnerBank: test.currentBankId,
				Status:      test.currentStatus,
			}
			err := transfer.SetPartnerBank(test.targetBankId)
			if err != nil && !test.expectError {
				t.Errorf("didn't expect error, but received one: %v", err)
			} else if err == nil && test.expectError {
				t.Errorf("expected error")
			}
			if err == nil && !test.expectError && (transfer.PartnerBank == nil || *transfer.PartnerBank != test.targetBankId) {
				t.Errorf("transfer.PartnerBank is not targetBankId")
			}
		})
	}
}
