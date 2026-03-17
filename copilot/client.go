// Package copilot encapsula a inicialização e gerência do cliente do Copilot SDK.
// O SDK atua como cliente JSON-RPC comunicando-se com o executável Copilot CLI
// rodando em "server mode". A autenticação é feita via COPILOT_GITHUB_TOKEN,
// GH_TOKEN, GITHUB_TOKEN ou credenciais locais (copilot auth login).
//
// IMPORTANTE: Classic personal access tokens (ghp_) NÃO são suportados pelo CLI.
// Use fine-grained PATs (github_pat_...) com permissão "Copilot Requests",
// ou OAuth tokens obtidos via `copilot login` ou `gh auth login`.
package copilot

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	sdk "github.com/github/copilot-sdk/go"
)

// tokenEnvVars lista as variáveis de ambiente verificadas, em ordem de prioridade.
var tokenEnvVars = []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"}

// resolveToken retorna o primeiro token encontrado nas variáveis de ambiente.
func resolveToken() string {
	for _, env := range tokenEnvVars {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return ""
}

// Manager gerencia o ciclo de vida do cliente Copilot SDK.
// É thread-safe e pode ser compartilhado entre handlers.
type Manager struct {
	client *sdk.Client
	mu     sync.RWMutex
}

// NewManager cria um Manager com o cliente SDK já instanciado (mas não iniciado).
// O token é lido das variáveis de ambiente e passado explicitamente ao SDK
// via ClientOptions.GitHubToken para garantir que o CLI o receba.
func NewManager() *Manager {
	token := resolveToken()

	// Validação de token na inicialização.
	if token == "" {
		slog.Warn("copilot: nenhum token encontrado (COPILOT_GITHUB_TOKEN / GH_TOKEN / GITHUB_TOKEN)")
		slog.Warn("copilot: defina um fine-grained PAT (github_pat_...) com permissão 'Copilot Requests'")
	} else {
		// Detectar tipo de token para ajudar no diagnóstico.
		prefix := token
		if len(prefix) > 10 {
			prefix = prefix[:10] + "..."
		}
		switch {
		case strings.HasPrefix(token, "ghp_"):
			slog.Error("copilot: token INVÁLIDO — Classic PAT (ghp_) NÃO é suportado pelo Copilot CLI")
			slog.Error("copilot: use um fine-grained PAT (github_pat_...) com permissão 'Copilot Requests'")
		case strings.HasPrefix(token, "github_pat_"):
			slog.Info("copilot: token detectado — fine-grained PAT (github_pat_...)")
		case strings.HasPrefix(token, "gho_"):
			slog.Info("copilot: token detectado — OAuth token (gho_...)")
		case strings.HasPrefix(token, "ghu_"):
			slog.Info("copilot: token detectado — user-to-server token (ghu_...)")
		default:
			slog.Info("copilot: token detectado", "prefix", prefix)
		}
	}

	opts := &sdk.ClientOptions{
		GitHubToken: token,
	}

	return &Manager{
		client: sdk.NewClient(opts),
	}
}

// Start inicializa o cliente SDK (dispara o servidor RPC interno).
// Deve ser chamado uma única vez na inicialização da aplicação.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	slog.Info("copilot: iniciando cliente SDK (JSON-RPC)...")
	if err := m.client.Start(ctx); err != nil {
		return fmt.Errorf("copilot: falha ao iniciar cliente: %w", err)
	}
	slog.Info("copilot: cliente SDK iniciado com sucesso")
	return nil
}

// Stop encerra o cliente SDK e o processo RPC subjacente.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	slog.Info("copilot: encerrando cliente SDK...")
	if err := m.client.Stop(); err != nil {
		slog.Error("copilot: erro ao encerrar cliente", "error", err)
	}
}

// Client retorna o cliente SDK subjacente para uso nos handlers.
// O chamador NÃO deve chamar Start/Stop diretamente no client retornado.
func (m *Manager) Client() *sdk.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client
}
