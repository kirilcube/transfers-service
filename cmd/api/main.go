package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"transfers/internal/adapters/kafka"
	"transfers/internal/repository"
	"transfers/internal/service"

	"github.com/jackc/pgx/v5/pgxpool"
)

const DATABASE_URL = ""

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := pgxpool.New(ctx, DATABASE_URL+"?pool_max_conns=12")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	transfersRepo := repository.NewTransferRepo(db)
	ts := service.NewTransfersService(transfersRepo)

	var wg sync.WaitGroup

	clearedSctHandler := kafka.NewSCTClearedHandler(ts)
	maxConcurrentSctHandling := 200
	wg.Go(func() {
		kafka.RunKafkaConsumer(ctx, "sct.cleared", "group_sct_cleared", clearedSctHandler, maxConcurrentSctHandling)
	})
}
