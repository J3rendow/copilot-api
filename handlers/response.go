package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// JSON escreve uma resposta JSON com status code e payload.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("handlers: falha ao serializar JSON", "error", err)
	}
}

// ErrorResponse é o payload padrão para respostas de erro.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// JSONError escreve uma resposta de erro JSON.
func JSONError(w http.ResponseWriter, status int, msg string, details string) {
	JSON(w, status, ErrorResponse{Error: msg, Details: details})
}
