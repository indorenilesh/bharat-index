package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/indorenilesh/bharat-index/internal/market"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: market-service <stock|calc-index|ui|admin>")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "stock":
		err = market.RunStock(ctx)
	case "calc-index":
		err = market.RunIndex(ctx)
	case "ui":
		err = market.RunUI(ctx)
	case "admin":
		err = market.RunAdmin(ctx)
	default:
		err = fmt.Errorf("unknown service mode %q", os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
}
