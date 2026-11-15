package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/ujjwal/gopherq/internal/api"
	"github.com/ujjwal/gopherq/internal/config"
	"github.com/ujjwal/gopherq/internal/handler"
	"github.com/ujjwal/gopherq/internal/handler/demo"
	"github.com/ujjwal/gopherq/internal/store/memory"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	reg := handler.NewRegistry()
	reg.Register("email", demo.Email, demo.ValidateEmail)
	reg.Register("flaky", demo.Flaky, demo.ValidateFlaky)
	reg.Register("webhook", demo.NewWebhook(), demo.ValidateWebhook)

	server := api.NewServer(memory.New(), reg)

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
