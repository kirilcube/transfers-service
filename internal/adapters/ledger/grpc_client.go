package ledger

import (
	"context"
	"fmt"
	"sync"
	"time"
	"transfers/internal/domain"
)

type GrpcClient struct {
	m  map[string]struct{}
	mu sync.Mutex
}

func NewLedgerGrpcClient() *GrpcClient {
	return &GrpcClient{
		m: make(map[string]struct{}),
	}
}

func (c *GrpcClient) ReserveFunds(ctx context.Context, t *domain.Transfer) error {
	// ledger's service will ignore same keys
	idempotencyKey := t.DeriveIdempotencyKey(domain.LedgerReserveFundsEvent)
	ok := c.mockServersDeduplication(idempotencyKey)
	if !ok {
		return nil
	}
	// simulating network round-trip
	timer := time.NewTimer(100 * time.Microsecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}

	fmt.Printf("DEBIT liability:merchant{%s} AMOUNT %v\n", t.MerchantId.String(), t.TotalDebit)
	fmt.Printf("CREDIT uncleared:tsf_id{%s} AMOUNT %v\n", t.Id.String(), t.PrincipalAmount)
	fmt.Printf("CREDIT revenue:fees AMOUNT %v\n", t.FeeAmount)
	return nil
}

func (c *GrpcClient) SettleFunds(ctx context.Context, t *domain.Transfer) error {
	// ledger's service will ignore same keys
	idempotencyKey := t.DeriveIdempotencyKey(domain.LedgerSettleFundsEvent)
	ok := c.mockServersDeduplication(idempotencyKey)
	if !ok {
		return nil
	}

	// simulating network round-trip
	timer := time.NewTimer(100 * time.Microsecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}

	fmt.Printf("DEBIT uncleared:tsf_id{%s} AMOUNT %v\n", t.Id.String(), t.PrincipalAmount)
	fmt.Printf("CREDIT asset:partner_bank{%s} AMOUNT %v\n", t.PartnerBank.Code, t.PrincipalAmount)
	return nil
}

func (c *GrpcClient) mockServersDeduplication(idemKey string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[idemKey]; ok {
		return false
	}
	c.m[idemKey] = struct{}{}
	return true
}
