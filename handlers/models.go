package handlers

import (
	"log/slog"
	"net/http"

	"github.com/local/api-rest-copilot/copilot"
)

// ModelsHandler retorna um handler para GET /models.
// Consulta dinamicamente os modelos disponíveis no Copilot SDK
// e os classifica entre free_0x e premium_request.
func ModelsHandler(mgr *copilot.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			JSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "use GET")
			return
		}

		ctx := r.Context()
		client := mgr.Client()

		// Consulta os modelos disponíveis via SDK (JSON-RPC -> Copilot CLI).
		models, err := client.ListModels(ctx)
		if err != nil {
			slog.Error("handlers/models: falha ao listar modelos", "error", err)
			JSONError(w, http.StatusBadGateway, "copilot_error", err.Error())
			return
		}

		// Extrai os IDs dos modelos retornados pelo SDK.
		ids := make([]string, 0, len(models))
		for _, m := range models {
			ids = append(ids, m.ID)
		}

		slog.Info("handlers/models: IDs recebidos do SDK", "ids", ids)

		// Classifica os modelos por tier (free vs premium).
		resp := copilot.ClassifyModels(ids)

		slog.Info("handlers/models: modelos listados", "total", resp.Total)
		JSON(w, http.StatusOK, resp)
	}
}
