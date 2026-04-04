package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/local/api-rest-copilot/copilot"

	sdk "github.com/github/copilot-sdk/go"
)

// OpenAIChatHandler retorna um handler para POST /v1/chat/completions.
// Aceita requests no formato OpenAI e traduz para sessões do Copilot SDK.
// Suporta tanto respostas síncronas (stream: false) quanto SSE (stream: true).
func OpenAIChatHandler(mgr *copilot.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Aceita Authorization header sem validar — auth é via SDK/CLI.
		// Clientes OpenAI enviam "Bearer sk-xxx" por padrão.

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB max
		if err != nil {
			openAIError(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
			return
		}
		defer r.Body.Close()

		var req OpenAIChatRequest
		if err := json.Unmarshal(body, &req); err != nil {
			openAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON: "+err.Error())
			return
		}

		if len(req.Messages) == 0 {
			openAIError(w, http.StatusBadRequest, "invalid_request_error", "messages array is required and must not be empty")
			return
		}
		if req.Model == "" {
			req.Model = "gpt-5-mini"
		}

		if req.Stream {
			handleOpenAIStream(w, r, mgr, req)
		} else {
			handleOpenAISync(w, r, mgr, req)
		}
	}
}

// handleOpenAISync processa um request síncrono (stream: false).
func handleOpenAISync(w http.ResponseWriter, r *http.Request, mgr *copilot.Manager, req OpenAIChatRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	systemPrompt, prompt := buildPromptFromMessages(req.Messages)

	client := mgr.Client()
	sessionCfg := &sdk.SessionConfig{
		Model:               req.Model,
		OnPermissionRequest: sdk.PermissionHandler.ApproveAll,
	}
	if systemPrompt != "" {
		sessionCfg.SystemMessage = &sdk.SystemMessageConfig{
			Mode:    "replace",
			Content: systemPrompt,
		}
	}

	session, err := client.CreateSession(ctx, sessionCfg)
	if err != nil {
		slog.Error("handlers/openai: falha ao criar sessão", "model", req.Model, "error", err)
		openAIError(w, http.StatusBadGateway, "server_error", "failed to create session: "+err.Error())
		return
	}
	defer session.Destroy()

	// Captura usage do evento assistant.usage se disponível.
	var inputTokens, outputTokens int
	unsubUsage := session.On(func(event sdk.SessionEvent) {
		if event.Type == sdk.AssistantUsage {
			if event.Data.InputTokens != nil {
				inputTokens = int(*event.Data.InputTokens)
			}
			if event.Data.OutputTokens != nil {
				outputTokens = int(*event.Data.OutputTokens)
			}
		}
	})
	defer unsubUsage()

	event, err := session.SendAndWait(ctx, sdk.MessageOptions{Prompt: prompt})
	if err != nil {
		slog.Error("handlers/openai: falha no SendAndWait", "error", err)
		openAIError(w, http.StatusBadGateway, "server_error", "copilot error: "+err.Error())
		return
	}

	content := ""
	if event != nil && event.Data.Content != nil {
		content = *event.Data.Content
	}

	// Verifica se houve tool_calls na resposta.
	var toolCalls []OpenAIToolCall
	finishReason := "stop"
	if event != nil && len(event.Data.ToolRequests) > 0 {
		finishReason = "tool_calls"
		for _, tr := range event.Data.ToolRequests {
			args := "{}"
			if tr.Arguments != nil {
				if b, err := json.Marshal(tr.Arguments); err == nil {
					args = string(b)
				}
			}
			toolCalls = append(toolCalls, OpenAIToolCall{
				ID:   tr.ToolCallID,
				Type: "function",
				Function: OpenAIFuncCall{
					Name:      tr.Name,
					Arguments: args,
				},
			})
		}
	}

	id := fmt.Sprintf("chatcmpl-copilot-%d", time.Now().UnixNano())

	msg := &OpenAIMessage{
		Role:    "assistant",
		Content: content,
	}
	if len(toolCalls) > 0 {
		msg.Content = nil
		msg.ToolCalls = toolCalls
	}

	slog.Info("handlers/openai: resposta síncrona gerada",
		"model", req.Model,
		"content_len", len(content),
		"tool_calls", len(toolCalls),
	)

	JSON(w, http.StatusOK, OpenAIChatResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []OpenAIChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: &finishReason,
			},
		},
		Usage: OpenAIUsage{
			PromptTokens:     inputTokens,
			CompletionTokens: outputTokens,
			TotalTokens:      inputTokens + outputTokens,
		},
	})
}

// handleOpenAIStream processa um request com streaming SSE (stream: true).
func handleOpenAIStream(w http.ResponseWriter, r *http.Request, mgr *copilot.Manager, req OpenAIChatRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		openAIError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	systemPrompt, prompt := buildPromptFromMessages(req.Messages)

	client := mgr.Client()
	sessionCfg := &sdk.SessionConfig{
		Model:               req.Model,
		Streaming:           true,
		OnPermissionRequest: sdk.PermissionHandler.ApproveAll,
	}
	if systemPrompt != "" {
		sessionCfg.SystemMessage = &sdk.SystemMessageConfig{
			Mode:    "replace",
			Content: systemPrompt,
		}
	}

	session, err := client.CreateSession(ctx, sessionCfg)
	if err != nil {
		slog.Error("handlers/openai_stream: falha ao criar sessão", "error", err)
		openAIError(w, http.StatusBadGateway, "server_error", "failed to create session: "+err.Error())
		return
	}
	defer session.Destroy()

	// Headers SSE — devem ser enviados antes de qualquer dado.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx proxy

	id := fmt.Sprintf("chatcmpl-copilot-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	// Envia o primeiro chunk com role.
	firstChunk := OpenAIChatResponse{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   req.Model,
		Choices: []OpenAIChoice{
			{
				Index: 0,
				Delta: &OpenAIDelta{Role: "assistant"},
			},
		},
	}
	writeSSE(w, flusher, firstChunk)

	// Canal para receber eventos do SDK.
	type streamEvent struct {
		content      string
		finishReason *string
		err          string
	}
	events := make(chan streamEvent, 128)

	unsubscribe := session.On(func(event sdk.SessionEvent) {
		switch event.Type {
		case sdk.AssistantMessageDelta:
			if event.Data.DeltaContent != nil && *event.Data.DeltaContent != "" {
				events <- streamEvent{content: *event.Data.DeltaContent}
			}
		case sdk.SessionIdle:
			stop := "stop"
			events <- streamEvent{finishReason: &stop}
		case sdk.SessionError:
			errMsg := "unknown_error"
			if event.Data.Message != nil {
				errMsg = *event.Data.Message
			}
			events <- streamEvent{err: errMsg}
		}
	})
	defer unsubscribe()

	_, err = session.Send(ctx, sdk.MessageOptions{Prompt: prompt})
	if err != nil {
		slog.Error("handlers/openai_stream: falha ao enviar mensagem", "error", err)
		// Já enviamos headers SSE, então enviamos erro como chunk final.
		errStop := "stop"
		errChunk := OpenAIChatResponse{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: req.Model,
			Choices: []OpenAIChoice{{Index: 0, Delta: &OpenAIDelta{Content: "[error: " + err.Error() + "]"}, FinishReason: &errStop}},
		}
		writeSSE(w, flusher, errChunk)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	// Loop de streaming SSE.
	for {
		select {
		case ev := <-events:
			if ev.err != "" {
				// Erro — envia como conteúdo e finaliza.
				stop := "stop"
				errChunk := OpenAIChatResponse{
					ID: id, Object: "chat.completion.chunk", Created: created, Model: req.Model,
					Choices: []OpenAIChoice{{Index: 0, Delta: &OpenAIDelta{Content: "[error: " + ev.err + "]"}, FinishReason: &stop}},
				}
				writeSSE(w, flusher, errChunk)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			if ev.finishReason != nil {
				// Fim do stream — chunk final com finish_reason.
				doneChunk := OpenAIChatResponse{
					ID: id, Object: "chat.completion.chunk", Created: created, Model: req.Model,
					Choices: []OpenAIChoice{{Index: 0, Delta: &OpenAIDelta{}, FinishReason: ev.finishReason}},
				}
				writeSSE(w, flusher, doneChunk)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				slog.Info("handlers/openai_stream: streaming finalizado", "model", req.Model)
				return
			}

			// Token delta — envia como chunk SSE.
			chunk := OpenAIChatResponse{
				ID: id, Object: "chat.completion.chunk", Created: created, Model: req.Model,
				Choices: []OpenAIChoice{{Index: 0, Delta: &OpenAIDelta{Content: ev.content}}},
			}
			writeSSE(w, flusher, chunk)

		case <-ctx.Done():
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
	}
}

// writeSSE serializa e escreve um chunk SSE.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// buildPromptFromMessages traduz o array de messages OpenAI para:
// - systemPrompt: conteúdo da mensagem system (para SessionConfig.SystemMessage)
// - prompt: concatenação das demais mensagens como prompt para o SDK
func buildPromptFromMessages(messages []OpenAIMessage) (systemPrompt, prompt string) {
	var systemParts []string
	var conversationParts []string

	for _, msg := range messages {
		text := extractTextContent(msg.Content)
		if text == "" {
			continue
		}

		switch msg.Role {
		case "system":
			systemParts = append(systemParts, text)
		case "user":
			conversationParts = append(conversationParts, text)
		case "assistant":
			conversationParts = append(conversationParts, "[Previous assistant response]: "+text)
		case "tool":
			toolID := msg.ToolCallID
			if toolID == "" {
				toolID = "unknown"
			}
			conversationParts = append(conversationParts, fmt.Sprintf("[Tool result for %s]: %s", toolID, text))
		}
	}

	systemPrompt = strings.Join(systemParts, "\n\n")
	prompt = strings.Join(conversationParts, "\n\n")
	return
}

// extractTextContent extrai o texto de um campo Content que pode ser:
// - string (caso comum)
// - []interface{} (array de ContentParts para multimodal)
func extractTextContent(content any) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var texts []string
		for _, part := range v {
			m, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if m["type"] == "text" {
				if t, ok := m["text"].(string); ok {
					texts = append(texts, t)
				}
			}
			// image_url parts são ignorados aqui — o SDK não suporta inline images via URL.
		}
		return strings.Join(texts, "\n")
	default:
		return fmt.Sprintf("%v", content)
	}
}
