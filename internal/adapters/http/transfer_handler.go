package http

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"transfers/internal/domain"
	"transfers/internal/service"

	"github.com/google/uuid"
)

type TransferHandler struct {
	svc *service.TransfersService
}

func NewTransferHandler(svc *service.TransfersService) *TransferHandler {
	return &TransferHandler{svc: svc}
}

func (h *TransferHandler) HandleCreateSCT(w http.ResponseWriter, r *http.Request) {
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		http.Error(w, "Idempotency-Key header is required", http.StatusBadRequest)
		return
	}

	var req struct {
		MerchantID      string `json:"merchant_id"`
		Iban            string `json:"iban"`
		Payee           string `json:"payee"`
		PrincipalAmount int64  `json:"amount"`
		Currency        string `json:"currency"`
		ReferenceText   string `json:"reference_text"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	merchantId, err := uuid.Parse(req.MerchantID)
	if err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	params := domain.CreateTransferParams{
		MerchantId:      merchantId,
		Iban:            req.Iban,
		Payee:           req.Payee,
		PrincipalAmount: req.PrincipalAmount,
		Currency:        req.Currency,
		ReferenceText:   req.ReferenceText,
	}

	transferId, err := h.svc.AcceptNewSCT(r.Context(), params, idemKey)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrIdempotencyConflict):
			http.Error(w, "Conflict: Idempotency key already used", http.StatusConflict)
		case errors.Is(err, domain.ErrVoPOverrideRequired):
			http.Error(w, "Verification of Payee failed", http.StatusUnprocessableEntity)
		case errors.Is(err, domain.ErrAntifraudFailed):
			http.Error(w, "Blocked by Antifraud", http.StatusForbidden)
		default:
			log.Printf("[ERR] failed SCT: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	respBody := map[string]string{
		"id":     transferId.String(),
		"status": "ACCEPTED",
	}
	json.NewEncoder(w).Encode(respBody)
}
