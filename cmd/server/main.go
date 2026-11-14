package main

import (
    "log"
    "net/http"

    "github.com/ujjwal/gopherq/internal/api"
)

func main() {
    server := api.NewServer()

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