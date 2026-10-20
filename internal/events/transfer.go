package events

import "github.com/google/uuid"

// In real system we'd use Apache Avro Schemes

type NewTransferEventPayload struct {
	IdempotencyKey string    `json:"idempotency_key"`
	Id             uuid.UUID `json:"id"`
	Iban           string    `json:"iban"`
	Payee          string    `json:"payee"`
	Amount         int64     `json:"amount"`
	Currency       string    `json:"currency"`
	MerchantId     uuid.UUID `json:"merchant_id"`
}

// In real system we'd use Apache Avro Schemes

type TransferSettledNotifyPayload struct {
	IdempotencyKey string    `json:"idempotency_key"`
	Id             uuid.UUID `json:"id"`
	MerchantId     uuid.UUID `json:"merchant_id"`
}

// In real system we'd use Apache Avro Schemes

type TransferCleared struct {
	Id uuid.UUID `json:"id"`
}
