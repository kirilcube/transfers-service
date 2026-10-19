package router

import (
	"context"
	"transfers/internal/domain"

	"github.com/google/uuid"
)

type Mock struct{}

func NewMock() *Mock {
	return &Mock{}
}

func (m *Mock) GetBank(ctx context.Context, transfer *domain.Transfer) (domain.Bank, error) {
	return domain.Bank{
		Code: "default",
		Id:   uuid.New(),
	}, nil
}
