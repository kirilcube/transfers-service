package http

import "net/http"

func RegisterRoutes(mux *http.ServeMux, transferHandler *TransferHandler, ledgerHandler *LedgerHandler) {
	mux.HandleFunc("POST /v1/transfers/sct", transferHandler.HandleCreateSCT)
	mux.HandleFunc("GET /v1/ledger/state", ledgerHandler.HandleGetState)
}
