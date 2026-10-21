package vop

import (
	"context"
	"transfers/internal/domain"
)

type Mock struct{}

func NewMock() *Mock {
	return &Mock{}
}

func (v *Mock) Check(ctx context.Context, iban string, payee string) (domain.VoPCode, string, error) {
	if iban == "fail_with_vop_no_match" {
		return domain.VoPNoMatch, "{}", nil
	}
	return domain.VoPMatch, "{}", nil
}
