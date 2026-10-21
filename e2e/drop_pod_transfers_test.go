package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

func TestDropPodTransfers(t *testing.T) {
	// --- CONFIG ---
	const apiHost = "http://localhost:8080"
	const secondsSendRequests = 20
	const rps = 100
	const secondsWaitForLedger = 60

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ledgerStateShouldBeZero(ctx, t, apiHost)

	db := connectToDb(ctx, t)
	defer db.Close()

	// --- TEST ---

	go func() {
		sendRequests(ctx, apiHost, secondsSendRequests, rps)
	}()
	time.Sleep(time.Second * (secondsSendRequests / 2))
	fmt.Println("[INFO] Killing the API container...")
	exec.Command("docker", "kill", "transfers-api-pod-1").Run()
	cancel()

	time.Sleep(time.Second * 1)

	ctx, cancel = context.WithCancel(context.Background())
	fmt.Println("[INFO] API Container killed.")
	fmt.Println("[INFO] Making transfers older, so sweeper picks them up right away.")
	_, err := db.Exec(ctx, `
		UPDATE sct_transfers 
		SET locked_at = locked_at - INTERVAL '5 minutes' 
		WHERE status IN ('CREATED', 'INITIALIZED')
	`)
	if err != nil {
		t.Fatalf("Failed db update: %v", err)
	}

	fmt.Println("[INFO] RESTARTING API container...")
	exec.Command("docker", "start", "transfers-api-pod-1").Run()

	time.Sleep(5 * time.Second)
	fmt.Println("[INFO] Resending same transfers...")
	defer cancel()
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
