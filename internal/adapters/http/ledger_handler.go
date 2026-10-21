package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"transfers/internal/domain"
)

type ledgerClient interface {
	GetState(ctx context.Context) (domain.LedgerState, error)
}
type LedgerHandler struct {
	client ledgerClient
}

func NewLedgerHandler(client ledgerClient) *LedgerHandler {
	return &LedgerHandler{
		client: client,
	}
}

func (h *LedgerHandler) HandleGetState(w http.ResponseWriter, r *http.Request) {
	state, err := h.client.GetState(r.Context())
	if err != nil {
		fmt.Printf("[ERR] HandleGetState | failed ledger client.GetState(): %v\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	respBody := map[string]int64{
		"total_transfers": state.TotalTransfers,
		"total_reserved":  state.TotalReserved,
		"total_settled":   state.TotalSettled,
		"total_fees":      state.TotalFees,
	}
	json.NewEncoder(w).Encode(respBody)
}
