// Command: API (control plane)
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"pointersvc/internal/client"
	"pointersvc/internal/httpserver"
	"pointersvc/internal/repo"
	"pointersvc/internal/service"
	"pointersvc/internal/utils"
)

func main() {
	router := chi.NewRouter()
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(15 * time.Second))
	router.Use(corsMiddleware)

	redis := repo.NewRedisClient()
	if err := redis.Ping(context.Background()); err != nil {
		log.Fatalf("redis ping failed: %v", err)
	}
	router.Use(utils.RequestLog())
	cli := client.NewLNExchangeClient()
	entrySvc := service.NewEntryService(redis)
	querySvc := service.NewQueryService(redis, cli, entrySvc)
	profitSvc := service.NewProfitService(redis, cli, querySvc)
	marginSvc := service.NewMarginService(redis)

	httpserver.RegisterOrderRoutes(router, redis, profitSvc, querySvc)
	httpserver.RegisterStrategiesRoutes(router, profitSvc, marginSvc)
	httpserver.RegisterEntryAutomationRoutes(router,redis, entrySvc)
	httpserver.RegisterProfitRoutes(router, redis, profitSvc)
	httpserver.RegisterMarginRoutes(router, redis, marginSvc)
	httpserver.RegisterConfigsRoutes(router, redis, querySvc)

	// Log a cada requisição (simples)
	loggedRouter := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		router.ServeHTTP(w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3073"
	}
	addr := "0.0.0.0:" + port
	log.Printf("🚀 Servidor iniciado em http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, loggedRouter))
}

func enableCORS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, csrf-token")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w, r)

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
