package repository

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
	"transfers/internal/adapters/billing"
	"transfers/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	ctx := context.Background()

	// 1. Start a temporary Postgres container
	container, err := tcpostgres.Run(ctx,
		"postgres:18.6-alpine",
		tcpostgres.WithDatabase("transfers_test"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.WithInitScripts(filepath.Join("..", "..", "migrations", "001_init.up.sql")), // Point to your actual migration file
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	// Clean up the container after the test finishes
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %v", err)
		}
	})

	// 2. Get the connection string
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	// 3. Connect pgx to the test container
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}

	t.Cleanup(func() { pool.Close() })

	return pool
}

func TestTransferRepo_Create(t *testing.T) {
	pool := setupTestDB(t)
	feeCalc := billing.NewFeeCalculator()
	repo := NewTransferRepo(pool, feeCalc)

	params := domain.CreateTransferParams{
		MerchantId:      uuid.New(),
		Iban:            "DE11111111111111111111",
		Payee:           "Test",
		PrincipalAmount: int64(12),
		Currency:        "EUR",
		ReferenceText:   "Test transfer",
	}
	idemKey := "test1"
	transfer, err := repo.Create(context.Background(), params, idemKey)
	if err != nil {
		t.Fatalf("failed call to .Create: %v", err)
	}
	if transfer.Status != domain.StatusCreated ||
		transfer.Currency != params.Currency ||
		transfer.MerchantId != params.MerchantId ||
		transfer.Iban != params.Iban ||
		transfer.Payee != params.Payee ||
		transfer.PrincipalAmount != params.PrincipalAmount {
		t.Errorf("transfer has incorrect values: %v", transfer)
	}
	_, err = repo.Create(context.Background(), params, idemKey)
	if err == nil {
		t.Errorf("Duplicate call was expected to fail with idempotency error but error is non-nil")
	} else if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Errorf("Duplicate call was expected to fail with idempotency error but error is %v", err)
	}
}
