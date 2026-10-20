package http

import "net/http"

func RegisterRoutes(mux *http.ServeMux, transferHandler *TransferHandler) {
	mux.HandleFunc("POST /v1/transfers/sct", transferHandler.HandleCreateSCT)
}
