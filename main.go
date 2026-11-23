package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"roudash/auth"
	"roudash/handlers"
	"roudash/k8s"
	"roudash/persist"
)

func main() {
	port := envOr("PORT", "8082")
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		client := &http.Client{Timeout: 2 * time.Second}
		res, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz")
		if err != nil {
			log.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			log.Fatalf("healthcheck: %s", res.Status)
		}
		return
	}

	store, err := persist.NewStore(os.Getenv("PERSIST_DIR"))
	if err != nil {
		log.Fatal(err)
	}
	manager := k8s.NewManager()
	if err := store.Restore(manager); err != nil {
		log.Printf("[persist] %v", err)
	}
	obfuscate := strings.EqualFold(envOr("API_OBFUSCATION", "true"), "true")
	router := handlers.New(manager, store, auth.NewRouteObfuscator(obfuscate))
	server := &http.Server{
		Addr:              net.JoinHostPort(envOr("LISTEN_ADDR", "127.0.0.1"), port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("RouDash listening on %s (obfuscated routes: %t, persist mode: %t)", server.Addr, obfuscate, store.IsEnabled())
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
