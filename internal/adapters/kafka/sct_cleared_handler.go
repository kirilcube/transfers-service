package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"transfers/internal/service"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
)

type SCTClearedHandler struct {
	svc *service.TransfersService
}

var _ Handler = &SCTClearedHandler{}

func NewSCTClearedHandler(svc *service.TransfersService) *SCTClearedHandler {
	h := SCTClearedHandler{
		svc: svc,
	}
	return &h
}

func (h *SCTClearedHandler) Handle(ctx context.Context, record *kgo.Record) error {
	var payload struct {
		Id uuid.UUID `json:"id"`
	}
	err := json.Unmarshal(record.Value, &payload)
	if err != nil {
		return fmt.Errorf("json.unmarshaling: %w", err)
	}
	return h.svc.SettleSCT(ctx, payload.Id)
}
