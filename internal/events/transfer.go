package events

import "github.com/google/uuid"

type NewTransferEventPayload struct {
	IdempotencyKey string    `json:"idempotency_key"`
	Id             uuid.UUID `json:"id"`
	Iban           string    `json:"iban"`
	Payee          string    `json:"payee"`
	Amount         int64     `json:"amount"`
	Currency       string    `json:"currency"`
	MerchantId     uuid.UUID `json:"merchant_id"`
}
