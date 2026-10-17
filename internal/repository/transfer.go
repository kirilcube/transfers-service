package repository

import (
	"context"
	"errors"
	"fmt"
	"transfers/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TransferRepo struct {
	db *pgxpool.Pool
}

func NewTransferRepo(db *pgxpool.Pool) *TransferRepo {
	t := TransferRepo{
		db: db,
	}

	return &t
}

func (r *TransferRepo) Create(ctx context.Context, params domain.CreateTransferParams, idempotencyKey string) (*domain.Transfer, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx,
		`INSERT INTO sct_transfers
			(merchant_id, iban, payee, amount, currency, reference_text, locked_at)
		VALUES 
		    ($1, $2, $3, $4, $5, $6, clock_timestamp()) 
		RETURNING id, status, version`,
		params.MerchantId, params.Iban, params.Payee, params.Amount, params.Currency, params.ReferenceText,
	)
	var transferID uuid.UUID
	var status domain.TransferStatus
	var version int
	err = row.Scan(&transferID, &status, &version)
	if err != nil {
		return nil, fmt.Errorf("failed to insert transfer row: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO idempotency_records 
    	(idempotency_key, merchant_id, transfer_id)
		VALUES ($1, $2, $3)
    	`, idempotencyKey, params.MerchantId, transferID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, domain.ErrIdempotencyConflict
		}
		return nil, fmt.Errorf("failed to insert idempotency record: %w", err)
	}
	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	transfer := domain.Transfer{
		Id:          transferID,
		Status:      status,
		PartnerBank: nil,
		MerchantId:  params.MerchantId,
		Iban:        params.Iban,
		Payee:       params.Payee,
		Amount:      params.Amount,
		Currency:    params.Currency,
		Version:     version,
	}

	return &transfer, nil
}

func (r *TransferRepo) UpdateStatus(ctx context.Context, t *domain.Transfer) error {
	tag, err := r.db.Exec(ctx, `UPDATE sct_transfers
		SET 
		    status = $1, 
		    version = version + 1,
		    locked_at = clock_timestamp(),
			updated_at = clock_timestamp()
		WHERE id = $2 AND version = $3
		`, t.Status, t.Id, t.Version)
	if err != nil {
		return fmt.Errorf("error updating status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrConcurrentModification
	}
	t.Version++

	return nil
}

func (r *TransferRepo) UpdateStatusAndPartnerBank(ctx context.Context, t *domain.Transfer) error {
	tag, err := r.db.Exec(ctx, `UPDATE sct_transfers
		SET 
		    partner_bank = $1,
		    status = $2,
		    version = version + 1,
		    
		    locked_at = clock_timestamp(),
			updated_at = clock_timestamp()
		WHERE id = $3 AND version = $4
		`, t.PartnerBank, t.Status, t.Id, t.Version)
	if err != nil {
		return fmt.Errorf("failed to update transfer row: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConcurrentModification
	}
	t.Version++

	return nil
}

func (r *TransferRepo) UpdateStatusAndOutbox(ctx context.Context, t *domain.Transfer, event *domain.OutboxEvent) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE sct_transfers
		SET 
		    status = $1,
			version = version + 1,
			locked_at = clock_timestamp(),
			updated_at = clock_timestamp()
		WHERE id = $2 AND version = $3`,
		t.Status, t.Id, t.Version,
	)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConcurrentModification
	}

	_, err = tx.Exec(ctx, `INSERT INTO outbox
		 (topic, key, headers, payload)
		 VALUES ($1, $2, $3, $4)`,
		event.Topic, event.Key, event.Headers, event.Payload)

	if err != nil {
		return fmt.Errorf("failed to write to outbox: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	t.Version++

	return nil
}
