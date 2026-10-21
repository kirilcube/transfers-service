package antifraud

import (
	"context"
	"sync/atomic"
	"transfers/internal/domain"
)

type Mock struct {
	isToFail atomic.Bool
}

func NewMock() *Mock {
	return &Mock{}
}

func (am *Mock) SetIsToFail(value bool) {
	am.isToFail.Store(value)
}

func (am *Mock) Check(ctx context.Context, transfer *domain.Transfer) (bool, error) {
	if am.isToFail.Load() || transfer.Payee == "FRAUD" {
		return false, nil
	}
	return true, nil
}
