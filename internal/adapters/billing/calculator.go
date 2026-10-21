package billing

import (
	"context"

	"github.com/google/uuid"
)

type FeeCalculator struct{}

func NewFeeCalculator() *FeeCalculator {
	return &FeeCalculator{}
}

func (c *FeeCalculator) CalculateFee(ctx context.Context, merchantId uuid.UUID, Iban string, principalAmount int64, currency string) (int64, error) {
	return 0, nil
}
