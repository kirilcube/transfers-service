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
	db            *pgxpool.Pool
	feeCalculator domain.FeeCalculator
}

var _ domain.TransferRepository = &TransferRepo{}

func NewTransferRepo(db *pgxpool.Pool, feeCalculator domain.FeeCalculator) *TransferRepo {
	t := TransferRepo{
		db:            db,
		feeCalculator: feeCalculator,
	}

	return &t
}

func (r *TransferRepo) Create(ctx context.Context, params domain.CreateTransferParams, idempotencyKey string) (*domain.Transfer, error) {
	feeAmount, err := r.feeCalculator.CalculateFee(ctx, params.MerchantId, params.Iban, params.PrincipalAmount, params.Currency)
	totalDebit := params.PrincipalAmount + feeAmount

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx,
		`INSERT INTO sct_transfers
			(merchant_id, iban, payee, principal_amount, fee_amount, total_debit, currency, reference_text, locked_at)
		VALUES 
		    ($1, $2, $3, $4, $5, $6, $7, $8, clock_timestamp()) 
		RETURNING id, status, version`,
		params.MerchantId, params.Iban, params.Payee, params.PrincipalAmount, feeAmount, totalDebit, params.Currency, params.ReferenceText,
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
		Id:              transferID,
		Status:          status,
		PartnerBank:     nil,
		MerchantId:      params.MerchantId,
		Iban:            params.Iban,
		Payee:           params.Payee,
		PrincipalAmount: params.PrincipalAmount,
		FeeAmount:       feeAmount,
		TotalDebit:      totalDebit,
		Currency:        params.Currency,
		Version:         version,
	}

	return &transfer, nil
}

func (r *TransferRepo) GetById(ctx context.Context, id uuid.UUID) (*domain.Transfer, error) {
	query :=
		`SELECT id, status, partner_bank, merchant_id, iban, payee, 
			principal_amount, fee_amount, total_debit, currency, 
			vop_override, version
		FROM sct_transfers 
		WHERE id = $1`
	row := r.db.QueryRow(ctx, query, id.String())

	var bankId *uuid.UUID
	transfer := &domain.Transfer{}
	err := row.Scan(
		&transfer.Id,
		&transfer.Status,
		&bankId,
		&transfer.MerchantId,
		&transfer.Iban,
		&transfer.Payee,
		&transfer.PrincipalAmount,
		&transfer.FeeAmount,
		&transfer.TotalDebit,
		&transfer.Currency,
		&transfer.VoPOverride,
		&transfer.Version,
	)
	if bankId != nil {
		bank, err := r.GetBankById(*bankId)
		if err != nil {
			return nil, fmt.Errorf("get bank by id(%s): %w", bankId.String(), err)
		}
		transfer.PartnerBank = bank
	}

	if err != nil {
		return nil, fmt.Errorf("sql SELECT query: %w", err)
	}

	return transfer, nil
}

func (r *TransferRepo) GetBankById(id uuid.UUID) (*domain.Bank, error) {
	return &domain.Bank{
		Id:   id,
		Code: "default",
	}, nil
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
		`, t.PartnerBank.Id, t.Status, t.Id, t.Version)
	if err != nil {
		return fmt.Errorf("failed to update transfer row: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConcurrentModification
	}
	t.Version++

	return nil
}

func (r *TransferRepo) UpdateStatusAndVoPResponse(ctx context.Context, t *domain.Transfer, vopResponseBody string) error {
	query :=
		`UPDATE sct_transfers
		SET vop_response = $1,
		    status = $2,
			version = version + 1,
			locked_at = clock_timestamp(),
			updated_at = clock_timestamp()
		WHERE id = $3 AND version = $4`
	tag, err := r.db.Exec(ctx, query, vopResponseBody, t.Status, t.Id, t.Version)
	if err != nil {
		return fmt.Errorf("sql update query: %w", err)
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
		 (aggregatetype, aggregateid, type, headers, payload)
		 VALUES ($1, $2, $3, $4, $5)`,
		event.Topic, event.Key, event.Type, event.Headers, event.Payload)

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

func (r *TransferRepo) SelectForReconciliation(ctx context.Context) ([]*domain.Transfer, error) {
	// TODO: 5 minutes is hardcoded, move to ENV
	query := `
		WITH claimed AS (
			SELECT id
			FROM sct_transfers
			WHERE status IN ('CREATED', 'INITIALIZED')
			  AND locked_at < clock_timestamp() - INTERVAL '5 minutes'
			FOR UPDATE SKIP LOCKED
			LIMIT 100
		)
		UPDATE sct_transfers t
		SET 
			version = t.version + 1,
			locked_at = clock_timestamp(),
			updated_at = clock_timestamp()
		FROM claimed
		WHERE t.id = claimed.id
		RETURNING 
			t.id, t.status, t.partner_bank, t.merchant_id, t.iban, t.payee, 
			t.principal_amount, t.fee_amount, t.total_debit, t.currency, 
			t.vop_override, t.version
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("claim transfers for reconciliation: %w", err)
	}
	defer rows.Close()

	res := make([]*domain.Transfer, 0, 100)
	for rows.Next() {
		var bankId *uuid.UUID
		transfer := &domain.Transfer{}
		err = rows.Scan(
			&transfer.Id,
			&transfer.Status,
			&bankId,
			&transfer.MerchantId,
			&transfer.Iban,
			&transfer.Payee,
			&transfer.PrincipalAmount,
			&transfer.FeeAmount,
			&transfer.TotalDebit,
			&transfer.Currency,
			&transfer.VoPOverride,
			&transfer.Version,
		)
		if err != nil {
			fmt.Printf("[WARN] SWEEPER | failed to scan claimed transfer: %v, skipping\n", err)
			// prevent poisoned pill problem, it will be retried in 5 minutes
			continue
		}
		if bankId != nil {
			bank, err := r.GetBankById(*bankId)
			if err != nil {
				return nil, fmt.Errorf("get bank by id(%s): %w", bankId.String(), err)
			}
			transfer.PartnerBank = bank
		}
		res = append(res, transfer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over claimed transfers: %w", err)
	}

	return res, nil
}
