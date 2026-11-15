package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/ujjwal/gopherq/internal/api"
	"github.com/ujjwal/gopherq/internal/config"
	"github.com/ujjwal/gopherq/internal/store/memory"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	server := api.NewServer(memory.New())

	addr := fmt.Sprintf(":%d", cfg.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.Handler(),
	}

	log.Printf("GopherQ listening on %s", addr)

	if err := httpServer.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
