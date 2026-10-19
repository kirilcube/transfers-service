package antifraud

import (
	"context"
	"transfers/internal/domain"
)

type Mock struct{}

func NewMock() *Mock {
	return &Mock{}
}

func (am *Mock) Check(ctx context.Context, transfer *domain.Transfer) (bool, error) {
	if transfer.Payee == "FRAUD" {
		return false, nil
	}
	return true, nil
}
