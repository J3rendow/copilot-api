package copilot

import (
	"sort"
	"strings"
)

// ModelTier representa a classificação de custo de um modelo.
type ModelTier string

const (
	TierFree    ModelTier = "free_0x"
	TierPremium ModelTier = "premium_request"
)

// ClassifiedModel agrupa um modelo com sua classificação de custo.
type ClassifiedModel struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Tier       ModelTier `json:"tier"`
	Multiplier float64   `json:"multiplier"` // custo em premium requests por interação (planos pagos)
}

// ModelsResponse é o payload JSON retornado pelo endpoint GET /models.
type ModelsResponse struct {
	Free    []ClassifiedModel `json:"free"`
	Premium []ClassifiedModel `json:"premium"`
	Total   int               `json:"total"`
}

// modelMultiplier mapeia cada modelo ao seu multiplicador de premium requests
// em planos pagos (Pro/Pro+/Business/Enterprise).
// Multiplier 0 = modelo incluído (free), não consome premium requests.
// Fonte: https://docs.github.com/en/copilot/concepts/billing/copilot-requests#model-multipliers
// Última atualização: março 2026.
var modelMultiplier = map[string]float64{
	// ── Modelos incluídos (0× = gratuito em planos pagos) ──
	"gpt-4.1":      0,
	"gpt-4o":       0,
	"gpt-5-mini":   0,
	"raptor-mini":  0,

	// ── Modelos premium ──
	"claude-haiku-4.5":     0.33,
	"claude-sonnet-4":      1,
	"claude-sonnet-4.5":    1,
	"claude-sonnet-4.6":    1,
	"claude-opus-4.5":      3,
	"claude-opus-4.6":      3,
	"claude-opus-4.6-fast": 30,

	"gemini-2.5-pro":       1,
	"gemini-3-flash":       0.33,
	"gemini-3-pro":         1,
	"gemini-3-pro-preview": 1,
	"gemini-3.1-pro":       1,

	"gpt-5.1":              1,
	"gpt-5.1-codex":        1,
	"gpt-5.1-codex-mini":   0.33,
	"gpt-5.1-codex-max":    1,
	"gpt-5.2":              1,
	"gpt-5.2-codex":        1,
	"gpt-5.3-codex":        1,

	"grok-code-fast-1":     0.25,
}

// sortedPrefixes contém os prefixos do modelMultiplier ordenados do mais longo
// para o mais curto, usado no fallback de prefix matching.
var sortedPrefixes []string

func init() {
	sortedPrefixes = make([]string, 0, len(modelMultiplier))
	for k := range modelMultiplier {
		sortedPrefixes = append(sortedPrefixes, k)
	}
	sort.Slice(sortedPrefixes, func(i, j int) bool {
		return len(sortedPrefixes[i]) > len(sortedPrefixes[j])
	})
}

// lookupMultiplier procura o multiplicador de um modelo.
// Primeiro tenta match exato, depois fallback por prefixo (mais longo primeiro).
// Retorna (multiplier, found).
func lookupMultiplier(modelID string) (float64, bool) {
	// Match exato.
	if mult, ok := modelMultiplier[modelID]; ok {
		return mult, true
	}
	// Fallback: prefixo mais longo vence (ex: "gpt-4o" < "gpt-4o-mini").
	for _, prefix := range sortedPrefixes {
		if strings.HasPrefix(modelID, prefix) {
			return modelMultiplier[prefix], true
		}
	}
	return 1.0, false
}

// ClassifyTier retorna a classificação de um modelo baseado em seu ID.
func ClassifyTier(modelID string) ModelTier {
	mult, known := lookupMultiplier(modelID)
	if known && mult == 0 {
		return TierFree
	}
	return TierPremium
}

// getMultiplier retorna o multiplicador de um modelo. Modelos desconhecidos = 1.
func getMultiplier(modelID string) float64 {
	if mult, ok := lookupMultiplier(modelID); ok {
		return mult
	}
	return 1 // padrão conservador para modelos desconhecidos
}

// ClassifyModels recebe uma lista de IDs de modelo e retorna um ModelsResponse
// com os modelos agrupados por tier.
func ClassifyModels(modelIDs []string) ModelsResponse {
	resp := ModelsResponse{}
	for _, id := range modelIDs {
		tier := ClassifyTier(id)
		cm := ClassifiedModel{
			ID:         id,
			Name:       id,
			Tier:       tier,
			Multiplier: getMultiplier(id),
		}
		switch tier {
		case TierFree:
			resp.Free = append(resp.Free, cm)
		case TierPremium:
			resp.Premium = append(resp.Premium, cm)
		}
	}
	resp.Total = len(modelIDs)
	return resp
}
