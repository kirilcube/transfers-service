package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"transfers/internal/adapters/antifraud"
	"transfers/internal/adapters/billing"
	adapterHttp "transfers/internal/adapters/http"
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

	// transfers repo
	feeCalculator := billing.NewFeeCalculator()
	transfersRepo := repository.NewTransferRepo(db, feeCalculator)

	// transfers service
	ledgerClient := ledger.NewDBMock(db)
	vopChecker := vop.NewMock()
	antifraudChecker := antifraud.NewMock()
	routerClient := router.NewMock()
	ts := service.NewTransfersService(transfersRepo, vopChecker, antifraudChecker, routerClient, ledgerClient)

	var wg sync.WaitGroup

	// kafka consumer
	clearedSctHandler := kafka.NewSCTClearedHandler(ts)
	maxConcurrentSctHandling := 200
	wg.Go(func() {
		kafka.RunKafkaConsumer(quit, "sct.cleared", "group_sct_cleared", clearedSctHandler, maxConcurrentSctHandling)
	})

	transfersHandler := adapterHttp.NewTransferHandler(ts)
	mux := http.NewServeMux()
	adapterHttp.RegisterRoutes(mux, transfersHandler)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	serverCrashed := make(chan error, 1)
	go func() {
		log.Println("Starting HTTP server on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverCrashed <- err
		}
	}()
	select {
	case <-quit.Done():
		log.Println("[INFO] SIGTERM received. Shutting down gracefully...")
	case err := <-serverCrashed:
		log.Printf("[ERR] HTTP server crashed: %v\n", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERR] HTTP server forced shutdown: %v\n", err)
	}

	ts.Stop()
	wg.Wait()
	fmt.Printf("[INFO] Exited gracefully")
}
