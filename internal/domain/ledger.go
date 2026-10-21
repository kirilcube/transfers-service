package domain

type LedgerState struct {
	TotalTransfers int64
	TotalReserved  int64
	TotalSettled   int64
	TotalFees      int64
}
