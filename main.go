// API REST + WebSocket para automação local com GitHub Copilot SDK.
//
// Stack:
//   - Go 1.22+ (net/http com ServeMux melhorado)
//   - gorilla/websocket para streaming de tokens
//   - github.com/github/copilot-sdk/go como cliente JSON-RPC do Copilot CLI
//
// Endpoints:
//   GET  /models              → lista modelos classificados (free vs premium)
//   POST /chat                → chat síncrono (aguarda resposta completa)
//   WS   /chat/stream         → streaming de tokens via WebSocket
//   GET  /health              → health check
//   GET  /v1/models           → lista modelos (formato OpenAI)
//   POST /v1/chat/completions → chat síncrono ou SSE streaming (formato OpenAI)

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/local/api-rest-copilot/copilot"
	"github.com/local/api-rest-copilot/handlers"
	"github.com/local/api-rest-copilot/middleware"
)

func main() {
	// ── Structured logging ──────────────────────────────────────────
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// ── Configuração ────────────────────────────────────────────────
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// ── Copilot SDK Client ──────────────────────────────────────────
	// O Manager encapsula o ciclo de vida do cliente RPC.
	// A autenticação é resolvida por COPILOT_GITHUB_TOKEN, GH_TOKEN,
	// GITHUB_TOKEN ou credenciais locais (~/.config/github-copilot).
	mgr := copilot.NewManager()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := mgr.Start(ctx); err != nil {
		slog.Error("main: falha ao iniciar Copilot SDK", "error", err)
		os.Exit(1)
	}
	defer mgr.Stop()

	// ── Roteamento (net/http ServeMux Go 1.22+) ─────────────────────
	mux := http.NewServeMux()

	// Health check simples (útil para Docker HEALTHCHECK).
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	// Endpoints da API (formato nativo).
	mux.HandleFunc("GET /models", handlers.ModelsHandler(mgr))
	mux.HandleFunc("POST /chat", handlers.ChatHandler(mgr))
	mux.HandleFunc("/chat/stream", handlers.StreamHandler(mgr)) // WS (sem method prefix)

	// Endpoints OpenAI-compatible (/v1/...).
	// Permitem uso como drop-in replacement para clientes OpenAI
	// (OpenClaude, Aider, Continue, Open WebUI, LiteLLM, etc.).
	mux.HandleFunc("GET /v1/models", handlers.OpenAIModelsHandler(mgr))
	mux.HandleFunc("POST /v1/chat/completions", handlers.OpenAIChatHandler(mgr))

	// ── Middleware chain: Recoverer → CORS → Logger → Mux ───────────
	handler := middleware.Recoverer(
		middleware.CORS(
			middleware.Logger(mux),
		),
	)

	// ── Servidor HTTP ───────────────────────────────────────────────
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Minute, // streaming pode demorar
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	// Goroutine para graceful shutdown.
	go func() {
		<-ctx.Done()
		slog.Info("main: sinal recebido, encerrando servidor...")

		shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutCtx); err != nil {
			slog.Error("main: erro no shutdown", "error", err)
		}
	}()

	slog.Info("main: servidor iniciado", "port", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("main: erro fatal no servidor", "error", err)
		os.Exit(1)
	}

	slog.Info("main: servidor encerrado com sucesso")
}
