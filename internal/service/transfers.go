package service

import (
	"transfers/internal/domain"
	"transfers/internal/events"

	"context"
	"encoding/json"
	"fmt"
)

type TransfersService struct {
	repo         domain.TransferRepository
	vop          domain.VoPChecker
	antifraud    domain.AntifraudChecker
	router       domain.Router
	ledgerClient domain.LedgerClient
}

func (s *TransfersService) ExecuteSCT(ctx context.Context, transfer *domain.Transfer) error {
	err := s.InitializeAndVerify(ctx, transfer)
	if err != nil {
		return fmt.Errorf("InitializeAndVerify: %w", err)
	}

	err = s.HoldFundsAndOutbox(ctx, transfer)
	if err != nil {
		return fmt.Errorf("HoldFundsAndOutbox: %w", err)
	}

	return nil
}

func (s *TransfersService) InitializeAndVerify(ctx context.Context, transfer *domain.Transfer) error {
	if transfer.Status != domain.StatusCreated {
		return nil
	}
	if !transfer.VoPOverride {
		vopCode, vopResponseBody, err := s.vop.Check(ctx, transfer.Iban, transfer.Payee)
		if err != nil {
			return fmt.Errorf("vop checker error: %w", err)
		}
		if vopCode != domain.VoPMatch && vopCode != domain.VoPCloseMatch {
			err = transfer.TransitionTo(domain.StatusVoPNoMatch)
			if err != nil {
				return fmt.Errorf("transfer transition to StatusVoPNoMatch: %w", err)
			}
		}
		err = s.repo.UpdateStatusAndVoPResponse(ctx, transfer, vopResponseBody)
		if err != nil {
			return fmt.Errorf("repo update status and vop response body: %w", err)
		}
		if vopCode != domain.VoPMatch && vopCode != domain.VoPCloseMatch {
			return domain.ErrVoPOverrideRequired
		}
	}

	ok, err := s.antifraud.Check(ctx, transfer)
	if err != nil {
		return fmt.Errorf("antifraud check: %w", err)
	}
	if !ok {
		err = transfer.TransitionTo(domain.StatusAntifraudFailed)
		if err != nil {
			return fmt.Errorf("transfer transition to StatusAntifraudFailed: %w", err)
		}
		err = s.repo.UpdateStatus(ctx, transfer)
		if err != nil {
			return fmt.Errorf("repo update transfer status: %w", err)
		}
		return domain.ErrAntifraudFailed
	}

	bank, err := s.router.GetBank(ctx, transfer)
	if err != nil {
		return fmt.Errorf("router GetBank: %w", err)
	}
	err = transfer.SetPartnerBank(bank)
	if err != nil {
		return fmt.Errorf("transfer.SetPartnerBank: %w", err)
	}
	err = transfer.TransitionTo(domain.StatusInitialized)
	if err != nil {
		return fmt.Errorf("transfer.TransitionTo(StatusInitialized): %w", err)
	}
	err = s.repo.UpdateStatusAndPartnerBank(ctx, transfer)
	if err != nil {
		return fmt.Errorf("repo.UpdateStatusAndPartnerBank: %w", err)
	}
	return nil
}

func (s *TransfersService) HoldFundsAndOutbox(ctx context.Context, transfer *domain.Transfer) error {
	if transfer.Status != domain.StatusInitialized {
		return nil
	}
	err := s.ledgerClient.ReserveFunds(ctx, transfer)
	if err != nil {
		return fmt.Errorf("ledgerClient.ReserveFunds: %w", err)
	}

	err = transfer.TransitionTo(domain.StatusFundsHolded)
	if err != nil {
		return fmt.Errorf("transfer.TransitionTo(StatusFundsHolded): %w", err)
	}

	event, err := s.buildNewTransferEvent(transfer)
	if err != nil {
		return fmt.Errorf("buildBankAdapterEvent: %w", err)
	}
	err = s.repo.UpdateStatusAndOutbox(ctx, transfer, event)
	if err != nil {
		return fmt.Errorf("repo.UpdateStatusAndOutbox: %w", err)
	}

	return nil
}

func (s *TransfersService) buildNewTransferEvent(transfer *domain.Transfer) (*domain.OutboxEvent, error) {
	if transfer.PartnerBank == nil {
		return nil, fmt.Errorf("transfer.PartnerBank is nil")
	}
	idemKey := transfer.DeriveIdempotencyKey(domain.NewTransferEvent)
	payloadStruct := events.NewTransferEventPayload{
		IdempotencyKey: idemKey,
		Id:             transfer.Id,
		Iban:           transfer.Iban,
		Payee:          transfer.Payee,
		Amount:         transfer.PrincipalAmount,
		Currency:       transfer.Currency,
		MerchantId:     transfer.MerchantId,
	}

	payload, err := json.Marshal(payloadStruct)
	if err != nil {
		return nil, fmt.Errorf("json marshaling: %w", err)
	}
	event := domain.OutboxEvent{
		Topic:   fmt.Sprintf("new_sct.%s", transfer.PartnerBank.Code),
		Key:     transfer.Id.String(),
		Headers: []byte{},
		Payload: payload,
	}
	return &event, nil
}
