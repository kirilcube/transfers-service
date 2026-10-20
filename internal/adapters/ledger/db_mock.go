package ledger

import (
	"context"
	"fmt"
	"transfers/internal/domain"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DBMock struct {
	db *pgxpool.Pool
}

func NewDBMock(db *pgxpool.Pool) *DBMock {
	return &DBMock{
		db: db,
	}
}

func (c *DBMock) ReserveFunds(ctx context.Context, t *domain.Transfer) error {
	idemKey := t.DeriveIdempotencyKey(domain.LedgerReserveFundsEvent)
	query := `INSERT INTO mock_ledger_entries 
    		(idempotency_key, transfer_id, action, principal_amount, fee_amount)
		VALUES ($1, $2, 'RESERVE', $3, $4)`
	tag, err := c.db.Exec(ctx, query, idemKey, t.Id, t.PrincipalAmount, t.FeeAmount)
	if err != nil {
		return fmt.Errorf("insert failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// already inserted
		return nil
	}
	return nil
}

func (c *DBMock) SettleFunds(ctx context.Context, t *domain.Transfer) error {
	idemKey := t.DeriveIdempotencyKey(domain.LedgerReserveFundsEvent)
	query := `INSERT INTO mock_ledger_entries 
    		(idempotency_key, transfer_id, action, principal_amount, fee_amount)
		VALUES ($1, $2, 'SETTLE', $3, $4)`
	tag, err := c.db.Exec(ctx, query, idemKey, t.Id, t.PrincipalAmount, 0)
	if err != nil {
		return fmt.Errorf("insert failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// already inserted
		return nil
	}
	return nil
}

type State struct {
	TotalTransfers int64
	TotalReserved  int64
	TotalSettled   int64
	TotalFees      int64
}

func (c *DBMock) GetState(ctx context.Context) (State, error) {
	query := `
		SELECT 
			COUNT(DISTINCT transfer_id) AS total_transfers,
			COALESCE(SUM(principal_amount + fee_amount) FILTER (WHERE action = 'RESERVE'), 0) AS total_reserved,
			COALESCE(SUM(principal_amount) FILTER (WHERE action = 'SETTLE'), 0) AS total_settled,
			COALESCE(SUM(fee_amount) FILTER (WHERE action = 'RESERVE'), 0) AS total_fees
		FROM mock_ledger_entries
	`

	state := State{}

	err := c.db.QueryRow(ctx, query).Scan(
		&state.TotalTransfers,
		&state.TotalReserved,
		&state.TotalSettled,
		&state.TotalFees,
	)

	if err != nil {
		return State{}, fmt.Errorf("failed to query ledger state: %w", err)
	}

	return state, nil
}
