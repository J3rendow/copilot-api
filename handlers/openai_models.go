package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/local/api-rest-copilot/copilot"
)

// OpenAIModelsHandler retorna um handler para GET /v1/models.
// Reutiliza a lógica de ListModels do SDK e re-empacota no formato OpenAI.
func OpenAIModelsHandler(mgr *copilot.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		client := mgr.Client()

		models, err := client.ListModels(ctx)
		if err != nil {
			slog.Error("handlers/openai_models: falha ao listar modelos", "error", err)
			openAIError(w, http.StatusBadGateway, "server_error", err.Error())
			return
		}

		now := time.Now().Unix()
		data := make([]OpenAIModel, 0, len(models))
		for _, m := range models {
			data = append(data, OpenAIModel{
				ID:         m.ID,
				Object:     "model",
				Created:    now,
				OwnedBy:    "github-copilot",
				Permission: []string{},
				Root:       m.ID,
				Parent:     nil,
			})
		}

		slog.Info("handlers/openai_models: modelos listados", "total", len(data))
		JSON(w, http.StatusOK, OpenAIModelsResponse{
			Object: "list",
			Data:   data,
		})
	}
}

// openAIError escreve um erro no formato OpenAI.
func openAIError(w http.ResponseWriter, status int, errType, message string) {
	JSON(w, status, OpenAIErrorResponse{
		Error: OpenAIErrorDetail{
			Message: message,
			Type:    errType,
			Param:   nil,
			Code:    nil,
		},
	})
}
