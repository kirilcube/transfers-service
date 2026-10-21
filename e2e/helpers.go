//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var DATABASE_URL = "postgres://kiril:hi_sumup@localhost:5432/transfers?sslmode=disable&pool_max_conns=12"

var httpClient = &http.Client{
	Timeout: 5 * time.Second,
}

var merchantID = uuid.New()

type TransfersRequestBody struct {
	MerchantId    uuid.UUID `json:"merchant_id"`
	Iban          string    `json:"iban"`
	Payee         string    `json:"payee"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	ReferenceText string    `json:"reference_text"`
}

func sendTransfer(ctx context.Context, apiHost string, index int) {
	url := apiHost + "/v1/transfers/sct"

	transfer := &TransfersRequestBody{
		MerchantId:    merchantID,
		Iban:          "DE89 3704 0044 0532 0130 00",
		Payee:         "smbd",
		Amount:        int64(index),
		Currency:      "EUR",
		ReferenceText: fmt.Sprintf("index: %v", index),
	}
	body, err := json.Marshal(transfer)
	if err != nil {
		fmt.Printf("[ERR] failed to json marshal transfer (%v), err: %v\n", *transfer, err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		fmt.Printf("[ERR] failed to create request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("idem_%v", index))

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("[ERR] sendTransfer failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		errorBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			fmt.Printf("[ERR] sendTransfer response status: %v (could not read body)\n", resp.Status)
			return
		}
		if resp.StatusCode != 409 {
			fmt.Printf("[ERR] sendTransfer rejected - Status: %d, Message: %s\n", resp.StatusCode, string(errorBytes))
		}
		return
	}
}
func sendRequests(ctx context.Context, apiHost string, seconds int, rps int) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var wg sync.WaitGroup
	loopI := 0
loop:
	for {
		select {
		case <-ticker.C:
			for sent := 0; sent < rps; sent++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					sendTransfer(ctx, apiHost, index)
				}((loopI * rps) + sent + 1)
			}
			loopI++
			fmt.Printf("[INFO] sent %d requests %d-th time\n", rps, loopI)
			if loopI == seconds {
				break loop
			}
		case <-ctx.Done():
			break loop
		}
	}

	wg.Wait()
}

type LedgerState struct {
	TotalTransfers int64 `json:"total_transfers"`
	TotalReserved  int64 `json:"total_reserved"`
	TotalSettled   int64 `json:"total_settled"`
}

func (s LedgerState) isEmpty() bool {
	return s.TotalTransfers == 0 && s.TotalReserved == 0 && s.TotalSettled == 0
}

func getLedgerState(ctx context.Context, apiHost string) (LedgerState, error) {
	url := apiHost + "/v1/ledger/state"
	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer([]byte{}))
	if err != nil {
		return LedgerState{}, fmt.Errorf("create request: %v\n", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return LedgerState{}, fmt.Errorf("HTTP GET: %v\n", err)
	}
	defer resp.Body.Close()

	state := LedgerState{}
	err = json.NewDecoder(resp.Body).Decode(&state)
	if err != nil {
		return state, fmt.Errorf("json decoder: %w", err)
	}
	return state, nil
}
func printLedgerState(state LedgerState) {
	fmt.Printf("[INFO] Ledger state:\n- TotalTransfers: %v\n- TotalReserved: %v\n- TotalSettled: %v\n", state.TotalTransfers, state.TotalReserved, state.TotalSettled)
}
func assertLedgerState(state1, state2 LedgerState) bool {
	if state1.TotalSettled != state2.TotalSettled ||
		state1.TotalReserved != state2.TotalReserved ||
		state1.TotalTransfers != state2.TotalTransfers {
		return false
	}
	return true
}

func ledgerStateShouldBeZero(ctx context.Context, t *testing.T, apiHost string) {
	state, err := getLedgerState(ctx, apiHost)
	if err != nil {
		t.Fatalf("Failed GET LedgerState: %v", err)
	}
	if !state.isEmpty() {
		t.Fatalf("Ledger's state should be empty before running this test")
	}
}

func connectToDb(ctx context.Context, t *testing.T) *pgxpool.Pool {
	db, err := pgxpool.New(ctx, DATABASE_URL)
	if err != nil {
		t.Fatalf("Unable to connect to database: %v\n", err)
	}
	return db
}

func waitForLedgerState(ctx context.Context, apiHost string, expectedState LedgerState, tries int, t *testing.T) {
	fmt.Printf("Waiting for transfers to settle and ledger state to be as expected")
	var state LedgerState
	for i := 0; i < tries; i++ {
		var err error
		state, err = getLedgerState(ctx, apiHost)
		if err != nil {
			fmt.Printf("[ERR] failed GET LedgerState: %v\n", err)
			continue
		}
		if assertLedgerState(state, expectedState) {
			fmt.Printf("[SUCCESS] ledger state is as expected\n")
			printLedgerState(state)
			return
		}
		time.Sleep(time.Second)
	}
	t.Errorf("ledger state is not as expected.")
	fmt.Printf("Expected: \n")
	printLedgerState(expectedState)
	fmt.Printf("Got: \n")
	printLedgerState(state)
}
