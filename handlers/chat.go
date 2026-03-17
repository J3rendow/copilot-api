package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/local/api-rest-copilot/copilot"

	sdk "github.com/github/copilot-sdk/go"
)

// ChatRequest é o payload esperado no POST /chat.
type ChatRequest struct {
	Prompt          string `json:"prompt"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"` // "low", "medium", "high", "xhigh"
}

// ChatResponse é o payload retornado pelo POST /chat.
type ChatResponse struct {
	Model     string `json:"model"`
	Content   string `json:"content"`
	Reasoning string `json:"reasoning,omitempty"` // conteúdo do raciocínio (quando suportado)
}

// ChatHandler retorna um handler para POST /chat.
// Aceita JSON (application/json) ou multipart/form-data com arquivo opcional.
// Campos multipart: "prompt", "model", "reasoning_effort", "file" (opcional).
func ChatHandler(mgr *copilot.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use POST")
			return
		}

		var (
			req        ChatRequest
			attachment *sdk.Attachment
			cleanup    func()
		)

		ct := r.Header.Get("Content-Type")

		switch {
		case strings.HasPrefix(ct, "multipart/form-data"):
			// ── Multipart: campos de texto + arquivo opcional ──────────
			if err := r.ParseMultipartForm(maxFileSize); err != nil {
				JSONError(w, http.StatusBadRequest, "invalid_multipart", err.Error())
				return
			}

			req.Prompt = r.FormValue("prompt")
			req.Model = r.FormValue("model")
			req.ReasoningEffort = r.FormValue("reasoning_effort")

			// Processa arquivo opcional.
			file, header, err := r.FormFile("file")
			if err == nil {
				defer file.Close()

				if err := validateFileExtension(header.Filename); err != nil {
					JSONError(w, http.StatusBadRequest, "validation_error", err.Error())
					return
				}
				if header.Size > maxFileSize {
					JSONError(w, http.StatusBadRequest, "validation_error",
						"arquivo excede o limite de 5MB")
					return
				}

				tmpPath, cleanFn, err := saveTempFile(file, header.Filename)
				if err != nil {
					JSONError(w, http.StatusInternalServerError, "upload_error", err.Error())
					return
				}
				cleanup = cleanFn

				att := buildFileAttachment(tmpPath, header.Filename)
				attachment = &att
			}
			// err != nil && err == http.ErrMissingFile → sem arquivo, OK

		default:
			// ── JSON (backward compatible) ────────────────────────────
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				JSONError(w, http.StatusBadRequest, "invalid_body", err.Error())
				return
			}
			defer r.Body.Close()

			if err := json.Unmarshal(body, &req); err != nil {
				JSONError(w, http.StatusBadRequest, "invalid_json", err.Error())
				return
			}
		}

		// Cleanup do arquivo temporário após enviar a resposta.
		if cleanup != nil {
			defer cleanup()
		}

		if req.Prompt == "" {
			JSONError(w, http.StatusBadRequest, "validation_error", "prompt is required")
			return
		}
		if req.Model == "" {
			req.Model = "gpt-5-mini" // default econômico (free_0x)
		}

		// Valida reasoning_effort se informado.
		if req.ReasoningEffort != "" {
			switch req.ReasoningEffort {
			case "low", "medium", "high", "xhigh":
				// válido
			default:
				JSONError(w, http.StatusBadRequest, "validation_error",
					"reasoning_effort must be one of: low, medium, high, xhigh")
				return
			}
		}

		// Contexto independente do ReadTimeout do HTTP server.
		// O ReadTimeout (30s) cancela r.Context() após a leitura do body,
		// mas SendAndWait pode demorar muito mais para receber a resposta do LLM.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		client := mgr.Client()

		// Cria uma sessão efêmera para esta requisição.
		session, err := client.CreateSession(ctx, &sdk.SessionConfig{
			Model:               req.Model,
			ReasoningEffort:     req.ReasoningEffort,
			OnPermissionRequest: sdk.PermissionHandler.ApproveAll,
		})
		if err != nil {
			slog.Error("handlers/chat: falha ao criar sessão", "model", req.Model, "error", err)
			JSONError(w, http.StatusBadGateway, "copilot_session_error", err.Error())
			return
		}
		defer session.Destroy()

		// Monta as opções de mensagem, incluindo attachment se presente.
		msgOpts := sdk.MessageOptions{
			Prompt: req.Prompt,
		}
		if attachment != nil {
			msgOpts.Attachments = []sdk.Attachment{*attachment}
		}

		// Envia o prompt e aguarda a resposta completa (síncrono).
		// SendAndWait bloqueia até receber o evento assistant.turn_end.
		event, err := session.SendAndWait(ctx, msgOpts)
		if err != nil {
			slog.Error("handlers/chat: falha no SendAndWait", "error", err)
			JSONError(w, http.StatusBadGateway, "copilot_response_error", err.Error())
			return
		}

		content := ""
		if event != nil && event.Data.Content != nil {
			content = *event.Data.Content
		}

		// Extrai raciocínio (thinking) quando disponível.
		reasoning := ""
		if event != nil && event.Data.ReasoningText != nil {
			reasoning = *event.Data.ReasoningText
		}

		slog.Info("handlers/chat: resposta gerada",
			"model", req.Model,
			"content_len", len(content),
			"reasoning_len", len(reasoning),
			"reasoning_effort", req.ReasoningEffort,
		)

		JSON(w, http.StatusOK, ChatResponse{
			Model:     req.Model,
			Content:   content,
			Reasoning: reasoning,
		})
	}
}
