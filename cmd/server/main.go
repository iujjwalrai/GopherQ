package main

import (
    "log"
    "net/http"

    "github.com/ujjwal/gopherq/internal/api"
    "github.com/ujjwal/gopherq/internal/store/memory"
)

func main() {
    server := api.NewServer(memory.New())

    httpServer := &http.Server{
        Addr:    ":8080",
        Handler: server.Handler(),
    }

    log.Println("GopherQ listening on :8080")

    if err := httpServer.ListenAndServe(); err != nil &&
        err != http.ErrServerClosed {
        log.Fatal(err)
    }
}