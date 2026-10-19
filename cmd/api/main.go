package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"transfers/internal/adapters/antifraud"
	"transfers/internal/adapters/billing"
	"transfers/internal/adapters/kafka"
	"transfers/internal/adapters/ledger"
	"transfers/internal/adapters/router"
	"transfers/internal/repository"
	"transfers/internal/service"
	"transfers/internal/vop"

	"github.com/jackc/pgx/v5/pgxpool"
)

const DATABASE_URL = ""

func main() {
	quit, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(quit, DATABASE_URL+"?pool_max_conns=12")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	feeCalculator := billing.NewFeeCalculator()
	transfersRepo := repository.NewTransferRepo(db, feeCalculator)

	ledgerClient := ledger.NewLedgerGrpcClient()
	vopChecker := vop.NewMock()
	antifraudChecker := antifraud.NewMock()
	routerClient := router.NewMock()
	ts := service.NewTransfersService(transfersRepo, vopChecker, antifraudChecker, routerClient, ledgerClient)

	var wg sync.WaitGroup

	clearedSctHandler := kafka.NewSCTClearedHandler(ts)
	maxConcurrentSctHandling := 200
	wg.Go(func() {
		kafka.RunKafkaConsumer(quit, "sct.cleared", "group_sct_cleared", clearedSctHandler, maxConcurrentSctHandling)
	})
	<-quit.Done()
	ts.Stop()
	wg.Wait()
	fmt.Printf("[INFO] Exited gracefully")
}
