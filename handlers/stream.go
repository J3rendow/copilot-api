package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/local/api-rest-copilot/copilot"

	sdk "github.com/github/copilot-sdk/go"
)

// upgrader configura o upgrade HTTP -> WebSocket.
// CheckOrigin permite todas as origens para desenvolvimento local.
// Em produção, restrinja ao domínio esperado.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // TODO: restringir em produção
	},
}

// StreamRequest é a mensagem que o cliente WS envia para iniciar streaming.
type StreamRequest struct {
	Prompt          string          `json:"prompt"`
	Model           string          `json:"model"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"` // "low", "medium", "high", "xhigh"
	File            *FileAttachment `json:"file,omitempty"`             // arquivo anexo (base64)
}

// FileAttachment representa um arquivo codificado em base64 enviado via WebSocket.
type FileAttachment struct {
	Data     string `json:"data"`      // conteúdo em base64
	MimeType string `json:"mime_type"` // ex: "image/png", "text/plain"
	Name     string `json:"name"`      // nome com extensão, ex: "foto.png"
}

// StreamChunk é cada fragmento de texto enviado de volta ao cliente via WS.
// Type pode ser: "chunk" (texto), "reasoning" (thinking), "done", "error".
type StreamChunk struct {
	Type    string `json:"type"` // "chunk", "reasoning", "done", "error"
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// StreamHandler retorna um handler para WS /chat/stream.
// Aceita upgrade WebSocket, lê o prompt do cliente, invoca o agente
// em modo streaming usando session.On() + session.Send() e faz push
// dos chunks textuais via channel para o WebSocket.
func StreamHandler(mgr *copilot.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("handlers/stream: falha no upgrade WS", "error", err)
			return
		}
		defer conn.Close()

		// Configura timeouts para a conexão WS.
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		// Lê a primeira mensagem do cliente (prompt + modelo).
		_, msg, err := conn.ReadMessage()
		if err != nil {
			slog.Error("handlers/stream: falha ao ler mensagem WS", "error", err)
			return
		}

		var req StreamRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			writeWSChunk(conn, StreamChunk{Type: "error", Error: "invalid_json: " + err.Error()})
			return
		}

		if req.Prompt == "" {
			writeWSChunk(conn, StreamChunk{Type: "error", Error: "prompt is required"})
			return
		}
		if req.Model == "" {
			req.Model = "gpt-5-mini"
		}

		// Valida reasoning_effort se informado.
		if req.ReasoningEffort != "" {
			switch req.ReasoningEffort {
			case "low", "medium", "high", "xhigh":
				// válido
			default:
				writeWSChunk(conn, StreamChunk{Type: "error",
					Error: "reasoning_effort must be one of: low, medium, high, xhigh"})
				return
			}
		}

		// Valida e constrói attachment se presente.
		var (
			attachment *sdk.Attachment
			cleanup    func()
		)
		if req.File != nil {
			if req.File.Name == "" || req.File.MimeType == "" || req.File.Data == "" {
				writeWSChunk(conn, StreamChunk{Type: "error",
					Error: "file requer campos: data, mime_type, name"})
				return
			}
			if err := validateFileExtension(req.File.Name); err != nil {
				writeWSChunk(conn, StreamChunk{Type: "error", Error: err.Error()})
				return
			}
			// Decodifica base64 e valida tamanho.
			decoded, err := base64.StdEncoding.DecodeString(req.File.Data)
			if err != nil {
				writeWSChunk(conn, StreamChunk{Type: "error",
					Error: "base64 inválido no campo file.data"})
				return
			}
			if len(decoded) > maxFileSize {
				writeWSChunk(conn, StreamChunk{Type: "error",
					Error: "arquivo excede o limite de 5MB"})
				return
			}
			// Salva em arquivo temporário (SDK v0.1.29 só suporta "file" attachment).
			reader := bytes.NewReader(decoded)
			tmpPath, cleanFn, err := saveTempFile(reader, req.File.Name)
			if err != nil {
				writeWSChunk(conn, StreamChunk{Type: "error", Error: "upload_error: " + err.Error()})
				return
			}
			cleanup = cleanFn
			att := buildFileAttachment(tmpPath, req.File.Name)
			attachment = &att
		}
		if cleanup != nil {
			defer cleanup()
		}

		slog.Info("handlers/stream: iniciando streaming",
			"model", req.Model,
			"prompt_len", len(req.Prompt),
			"reasoning_effort", req.ReasoningEffort,
			"has_file", req.File != nil,
		)

		// Contexto com timeout para a operação de streaming completa.
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		client := mgr.Client()

		// Cria sessão com streaming habilitado.
		// Streaming: true faz o SDK emitir eventos assistant.message_delta
		// e assistant.reasoning_delta com DeltaContent contendo fragmentos.
		session, err := client.CreateSession(ctx, &sdk.SessionConfig{
			Model:               req.Model,
			Streaming:           true,
			ReasoningEffort:     req.ReasoningEffort,
			OnPermissionRequest: sdk.PermissionHandler.ApproveAll,
		})
		if err != nil {
			slog.Error("handlers/stream: falha ao criar sessão", "error", err)
			writeWSChunk(conn, StreamChunk{Type: "error", Error: "session_error: " + err.Error()})
			return
		}
		defer session.Destroy()

		// Canal para receber chunks de texto da goroutine de eventos.
		// Buffer de 128 para absorver rajadas de tokens sem bloquear o callback.
		chunks := make(chan StreamChunk, 128)

		// Subscreve nos eventos da sessão ANTES de enviar a mensagem.
		// O SDK dispara callbacks para cada evento recebido do servidor RPC.
		//
		// Com Streaming: true, os eventos relevantes são:
		//   - AssistantMessageDelta ("assistant.message_delta"): DeltaContent com fragmento
		//   - SessionIdle ("session.idle"): sinaliza fim completo da geração
		//   - SessionError ("session.error"): erro durante a geração
		//
		// Nota: AssistantMessage ("assistant.message") também é emitido ao final
		// com o conteúdo completo, mas para streaming usamos os deltas.
		unsubscribe := session.On(func(event sdk.SessionEvent) {
			switch event.Type {
			case sdk.AssistantMessageDelta:
				// Cada delta contém um fragmento incremental do texto.
				if event.Data.DeltaContent != nil && *event.Data.DeltaContent != "" {
					chunks <- StreamChunk{
						Type:    "chunk",
						Content: *event.Data.DeltaContent,
					}
				}

			case sdk.AssistantReasoningDelta:
				// Delta do raciocínio (thinking) — fragmentos incrementais.
				if event.Data.DeltaContent != nil && *event.Data.DeltaContent != "" {
					chunks <- StreamChunk{
						Type:    "reasoning",
						Content: *event.Data.DeltaContent,
					}
				}

			case sdk.SessionIdle:
				// Sessão ociosa — geração concluída, todos os tokens enviados.
				chunks <- StreamChunk{Type: "done"}

			case sdk.SessionError:
				errMsg := "unknown_error"
				if event.Data.Message != nil {
					errMsg = *event.Data.Message
				}
				chunks <- StreamChunk{Type: "error", Error: errMsg}
			}
		})
		defer unsubscribe()

		// Envia a mensagem (assíncrono — os eventos chegam via callback acima).
		msgOpts := sdk.MessageOptions{
			Prompt: req.Prompt,
		}
		if attachment != nil {
			msgOpts.Attachments = []sdk.Attachment{*attachment}
		}

		_, err = session.Send(ctx, msgOpts)
		if err != nil {
			slog.Error("handlers/stream: falha ao enviar mensagem", "error", err)
			writeWSChunk(conn, StreamChunk{Type: "error", Error: "send_error: " + err.Error()})
			return
		}

		// Loop principal: lê do channel de eventos e faz push via WebSocket.
		// Roda na goroutine do handler — não bloqueia outras conexões.
		for {
			select {
			case chunk := <-chunks:
				if err := writeWSChunk(conn, chunk); err != nil {
					slog.Error("handlers/stream: falha ao enviar chunk WS", "error", err)
					return
				}
				// Se é "done" ou "error", encerra o loop.
				if chunk.Type == "done" || chunk.Type == "error" {
					slog.Info("handlers/stream: streaming finalizado",
						"model", req.Model,
						"type", chunk.Type,
					)
					return
				}

			case <-ctx.Done():
				writeWSChunk(conn, StreamChunk{Type: "error", Error: "timeout"})
				return
			}
		}
	}
}

// writeWSChunk serializa e envia um StreamChunk pela conexão WebSocket.
func writeWSChunk(conn *websocket.Conn, chunk StreamChunk) error {
	data, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}
