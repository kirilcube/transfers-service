package service

import (
	"sync"
	"time"
	"transfers/internal/domain"
	"transfers/internal/events"

	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type TransfersService struct {
	repo             domain.TransferRepository
	vop              domain.VoPChecker
	antifraud        domain.AntifraudChecker
	router           domain.Router
	ledgerClient     domain.LedgerClient
	sweeperCtxCancel context.CancelFunc
	wg               sync.WaitGroup
}

func NewTransfersService(
	repo domain.TransferRepository,
	vop domain.VoPChecker,
	antifraud domain.AntifraudChecker,
	router domain.Router,
	ledgerClient domain.LedgerClient,
) *TransfersService {
	sweeperCtx, cancel := context.WithCancel(context.Background())
	s := &TransfersService{
		repo:             repo,
		vop:              vop,
		antifraud:        antifraud,
		router:           router,
		ledgerClient:     ledgerClient,
		sweeperCtxCancel: cancel,
	}
	s.wg.Go(func() {
		s.runSweeper(sweeperCtx)
	})
	return s
}

func (s *TransfersService) Stop() {
	s.sweeperCtxCancel()
	s.wg.Wait()
}

func (s *TransfersService) runSweeper(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var wg sync.WaitGroup

	for {
		select {
		case <-ticker.C:
			transfers, err := s.repo.SelectForReconciliation(ctx)
			if err != nil {
				fmt.Printf("[ERR] Sweeper | failed repo.SelectForReconciliation: %v\n", err)
			}
			for _, t := range transfers {
				wg.Go(func() {
					err := s.ExecuteSCT(context.Background(), t)
					if err != nil {
						fmt.Printf("[ERR] Sweeper | Failed reconciliation for transfer (%v): %v\n", t.Id, err)
					}
				})
			}
		case <-ctx.Done():
			timer := time.NewTimer(time.Second * 5)
			defer timer.Stop()
			goroutinesExited := make(chan struct{})
			go func() {
				wg.Wait()
				goroutinesExited <- struct{}{}
			}()

			select {
			case <-goroutinesExited:
				fmt.Println("Sweeper gracefully exited")
			case <-timer.C:
				fmt.Println("Sweeper forced to exit")
			}

			return
		}
	}
}

func (s *TransfersService) SettleSCT(ctx context.Context, transferId uuid.UUID) error {
	transfer, err := s.repo.GetById(ctx, transferId)
	if err != nil {
		return fmt.Errorf("selecting transfer by id from repo: %w", err)
	}
	if transfer.Status != domain.StatusSentForClearing {
		return fmt.Errorf("transfer's status is (%s) but should've been SentForClearing: %w", transfer.Status, err)
	}
	err = s.ledgerClient.SettleFunds(ctx, transfer)
	if err != nil {
		return fmt.Errorf("ledger.SettleFunds: %w", err)
	}
	err = transfer.TransitionTo(domain.StatusSettled)
	if err != nil {
		return fmt.Errorf("transfer transition to Status Settled: %w", err)
	}
	event, err := s.buildTransferSettledNotifyEvent(transfer)
	if err != nil {
		return fmt.Errorf("build transfer settled event: %w", err)
	}
	err = s.repo.UpdateStatusAndOutbox(ctx, transfer, event)
	if err != nil {
		return fmt.Errorf("repo update status and outbox: %w", err)
	}
	return nil
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

	err = transfer.TransitionTo(domain.StatusSentForClearing)
	if err != nil {
		return fmt.Errorf("transfer.TransitionTo(StatusSentForClearing): %w", err)
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
	// In real system we'd use Apache Avro Schemes
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

func (s *TransfersService) buildTransferSettledNotifyEvent(transfer *domain.Transfer) (*domain.OutboxEvent, error) {
	// In real system we'd use Apache Avro Schemes
	idemKey := transfer.DeriveIdempotencyKey(domain.TransferSettledEvent)
	payloadStruct := events.TransferSettledNotifyPayload{
		IdempotencyKey: idemKey,
		Id:             transfer.Id,
		MerchantId:     transfer.MerchantId,
	}

	payload, err := json.Marshal(payloadStruct)
	if err != nil {
		return nil, fmt.Errorf("json marshaling: %w", err)
	}
	event := domain.OutboxEvent{
		Topic:   fmt.Sprintf("notifications.users"),
		Key:     transfer.MerchantId.String(),
		Headers: []byte{},
		Payload: payload,
	}
	return &event, nil
}
