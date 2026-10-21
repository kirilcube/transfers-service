//go:build e2e

package e2e

import (
	"context"
	"testing"
)

func TestTransfers(t *testing.T) {
	// --- CONFIG ---
	const apiHost = "http://localhost:8080"
	const secondsSendRequests = 20
	const rps = 100
	const secondsWaitForLedger = 20

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ledgerStateShouldBeZero(ctx, t, apiHost)

	// --- TEST ---

	sendRequests(ctx, apiHost, secondsSendRequests, rps)

	// --- ASSERT LEDGER's STATE ---
	transfers := secondsSendRequests * rps

	// Sum of the first n consecutive positive integers formula
	sumTransfers := (int64(transfers) * (int64(transfers) + 1)) / 2

	expectedState := LedgerState{
		TotalTransfers: int64(transfers),
		TotalReserved:  sumTransfers,
		TotalSettled:   sumTransfers,
	}

	waitForLedgerState(ctx, apiHost, expectedState, secondsWaitForLedger, t)
}
