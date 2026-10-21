package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
	"transfers/internal/adapters/antifraud"
	"transfers/internal/adapters/billing"
	"transfers/internal/adapters/ledger"
	"transfers/internal/adapters/router"
	"transfers/internal/domain"
	"transfers/internal/repository"
	"transfers/internal/vop"

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

func getService(t *testing.T, opts ...Option) (*TransfersService, *ledger.DBMock) {
	db := setupTestDB(t)
	feeCalc := billing.NewFeeCalculator()

	ledgerClient := ledger.NewDBMock(db)
	repo := repository.NewTransferRepo(db, feeCalc)
	serviceOpts := serviceOpts{
		Antifraud: antifraud.NewMock(),
		VoP:       vop.NewMock(),
		Router:    router.NewMock(),
	}
	for _, op := range opts {
		op(&serviceOpts)
	}

	service := NewTransfersService(repo, serviceOpts.VoP, serviceOpts.Antifraud, serviceOpts.Router, ledgerClient)
	return service, ledgerClient
}

type serviceOpts struct {
	Antifraud domain.AntifraudChecker
	VoP       domain.VoPChecker
	Router    domain.Router
}
type Option func(opts *serviceOpts)

func TestTransfersService_AcceptNewSCT(t *testing.T) {
	ctx := context.Background()
	service, ledgerClient := getService(t)

	amount := int64(12)
	params := domain.CreateTransferParams{
		MerchantId:      uuid.New(),
		Iban:            "DE11111111111111111111",
		Payee:           "Test",
		PrincipalAmount: amount,
		Currency:        "EUR",
		ReferenceText:   "Test transfer",
	}
	idemKey := "test"

	// happy path
	_, err := service.AcceptNewSCT(ctx, params, idemKey)
	if err != nil {
		t.Fatalf("service.AcceptNewSCT failed: %v", err)
	}

	// idempotency error
	_, err = service.AcceptNewSCT(ctx, params, idemKey)
	if err == nil {
		t.Fatalf("service.AcceptNewSCT duplicate call error expected")
	} else if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("service.AcceptNewSCT duplicate call idempotency error expected, received: %v", err)
	}

	state, err := ledgerClient.GetState(ctx)
	if err != nil {
		t.Fatalf("ledger.GetState failed: %v", err)
	}
	// exactly once reserved funds
	if state.TotalReserved != amount ||
		state.TotalTransfers != 1 ||
		state.TotalSettled != 0 {
		t.Errorf("received ledger's state is incorrect: %v", state)
	}
}

func withAntifraud(antifraud domain.AntifraudChecker) Option {
	return func(opts *serviceOpts) {
		opts.Antifraud = antifraud
	}
}

func TestTransfersService_AcceptNewSCTAntifraudFail(t *testing.T) {
	ctx := context.Background()
	a := antifraud.NewMock()
	service, ledgerClient := getService(t, withAntifraud(a))

	a.SetIsToFail(true)

	amount := int64(12)
	params := domain.CreateTransferParams{
		MerchantId:      uuid.New(),
		Iban:            "DE11111111111111111111",
		Payee:           "Test",
		PrincipalAmount: amount,
		Currency:        "EUR",
		ReferenceText:   "Test transfer",
	}
	idemKey := "test"
	_, err := service.AcceptNewSCT(ctx, params, idemKey)
	if err == nil {
		t.Fatalf("service.AcceptNewSCT expected antifraud failed error")
	} else if !errors.Is(err, domain.ErrAntifraudFailed) {
		t.Fatalf("service.AcceptNewSCT expected antifraud failed error, received: %v", err)
	}

	state, err := ledgerClient.GetState(ctx)
	if err != nil {
		t.Fatalf("ledger.GetState failed: %v", err)
	}

	// no reserved funds
	if state.TotalReserved != 0 ||
		state.TotalTransfers != 0 ||
		state.TotalSettled != 0 {
		t.Errorf("received ledger's state is incorrect: %v", state)
	}
}

func TestTransfersService_AcceptNewSCTVoPNoMatch(t *testing.T) {
	ctx := context.Background()
	service, ledgerClient := getService(t)

	amount := int64(12)
	params := domain.CreateTransferParams{
		MerchantId:      uuid.New(),
		Iban:            "fail_with_vop_no_match",
		Payee:           "Test",
		PrincipalAmount: amount,
		Currency:        "EUR",
		ReferenceText:   "Test transfer",
	}
	idemKey := "test"
	_, err := service.AcceptNewSCT(ctx, params, idemKey)
	if err == nil {
		t.Fatalf("service.AcceptNewSCT expected vop_no_match error")
	} else if !errors.Is(err, domain.ErrVoPOverrideRequired) {
		t.Fatalf("service.AcceptNewSCT expected vop_no_match error, received: %v", err)
	}

	state, err := ledgerClient.GetState(ctx)
	if err != nil {
		t.Fatalf("ledger.GetState failed: %v", err)
	}

	// no reserved funds
	if state.TotalReserved != 0 ||
		state.TotalTransfers != 0 ||
		state.TotalSettled != 0 {
		t.Errorf("received ledger's state is incorrect: %v", state)
	}
}

func TestTransfersService_SettleSCT(t *testing.T) {
	ctx := context.Background()
	service, ledgerClient := getService(t)

	amount := int64(12)
	params := domain.CreateTransferParams{
		MerchantId:      uuid.New(),
		Iban:            "DE11111111111111111111",
		Payee:           "Test",
		PrincipalAmount: amount,
		Currency:        "EUR",
		ReferenceText:   "Test transfer",
	}
	idemKey := "test"

	id, err := service.AcceptNewSCT(ctx, params, idemKey)
	if err != nil {
		t.Fatalf("service.AcceptNewSCT failed: %v", err)
	}

	// happy path
	err = service.SettleSCT(ctx, id)
	if err != nil {
		t.Fatalf("service.SettleSCT failed: %v", err)
	}
	// second-delivery
	err = service.SettleSCT(ctx, id)
	if err != nil {
		// We expect err == nil if first Settle err == nil (idempotency)
		t.Fatalf("service.SettleSCT duplicate failed: %v", err)
	}

	state, err := ledgerClient.GetState(ctx)
	if err != nil {
		t.Fatalf("ledger.GetState failed: %v", err)
	}

	// exactly once reserved and settled
	if state.TotalReserved != amount ||
		state.TotalTransfers != 1 ||
		state.TotalSettled != amount {
		t.Errorf("received ledger's state is incorrect: %v", state)
	}
}
