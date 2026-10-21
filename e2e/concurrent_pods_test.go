package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestConcurrentPods(t *testing.T) {
	// --- CONFIG ---
	const podHost1 = "http://localhost:8080"
	const podHost2 = "http://localhost:8081"
	const secondsSendRequests = 30
	const rps = 100
	const secondsWaitForLedger = 20

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ledgerStateShouldBeZero(ctx, t, podHost1)

	// --- TEST ---

	var wg sync.WaitGroup
	wg.Go(func() {
		fmt.Printf("starting sending to pod1\n")
		sendRequests(ctx, podHost1, secondsSendRequests, rps)
		fmt.Printf("sending to pod1 finished\n")
	})
	wg.Go(func() {
		fmt.Printf("starting sending to pod2\n")
		sendRequests(ctx, podHost2, secondsSendRequests, rps)
		fmt.Printf("sending to pod2 finished\n")
	})
	wg.Wait()

	// --- ASSERT LEDGER's STATE ---
	transfers := secondsSendRequests * rps

	// Sum of the first n consecutive positive integers formula
	sumTransfers := (int64(transfers) * (int64(transfers) + 1)) / 2

	expectedState := LedgerState{
		TotalTransfers: int64(transfers),
		TotalReserved:  sumTransfers,
		TotalSettled:   sumTransfers,
	}

	waitForLedgerState(ctx, podHost1, expectedState, secondsWaitForLedger, t)
}
