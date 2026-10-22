package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"transfers/internal/repository"

	"transfers/internal/domain"
	"transfers/internal/service"

	"github.com/google/uuid"
)

type TransferHandler struct {
	svc  *service.TransfersService
	repo *repository.TransferRepo
}

func NewTransferHandler(svc *service.TransfersService, repo *repository.TransferRepo) *TransferHandler {
	return &TransferHandler{svc: svc, repo: repo}
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
	var respCode int
	respBody := make(map[string]string, 1)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrIdempotencyConflict):
			respCode, respBody, err := h.svc.GetCachedResponse(r.Context(), idemKey, merchantId)
			if err != nil {
				log.Printf("[ERR] failed get cached response: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(respCode)
			_, _ = w.Write([]byte(respBody))
			return
		case errors.Is(err, domain.ErrVoPOverrideRequired):
			respCode = http.StatusUnprocessableEntity
			respBody["message"] = "Verification of Payee failed. Override required"
		case errors.Is(err, domain.ErrAntifraudFailed):
			respCode = http.StatusForbidden
			respBody["message"] = "Blocked by Antifraud"
		default:
			log.Printf("[ERR] failed SCT: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		respCode = http.StatusAccepted
		respBody["message"] = "Accepted"
	}

	respBodyJson, err := json.Marshal(&respBody)
	if err != nil {
		fmt.Printf("[ERR] json marshal of response body (%v) failed: %v", respBody, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	err = h.repo.UpdateCachedResponse(r.Context(), idemKey, merchantId, transferId, respCode, string(respBodyJson))
	if err != nil {
		fmt.Printf("[ERR] Updating of cached response failed: %v", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(respCode)
	_, _ = w.Write(respBodyJson)
}
