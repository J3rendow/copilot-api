<div align="center">

# Copilot API

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=for-the-badge&logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=for-the-badge&logo=docker&logoColor=white)](https://www.docker.com/)
[![GitHub stars](https://img.shields.io/github/stars/J3rendow/copilot-api?style=for-the-badge)](https://github.com/J3rendow/copilot-api/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/J3rendow/copilot-api?style=for-the-badge)](https://github.com/J3rendow/copilot-api/network/members)
[![Last commit](https://img.shields.io/github/last-commit/J3rendow/copilot-api?style=for-the-badge)](https://github.com/J3rendow/copilot-api/commits/main)

**REST API + WebSocket** server built in **Go** on top of the official **GitHub Copilot SDK**.

Servidor **REST API + WebSocket** em **Go**, construído sobre o **SDK oficial do GitHub Copilot**.

[Português](#-português) · [English](#-english)

</div>

---

# 🇧🇷 Português

## Visão Geral

API HTTP e WebSocket que encapsula o [GitHub Copilot SDK](https://github.com/github/copilot-sdk) para oferecer chat síncrono, streaming de tokens e análise de arquivos.

```
Cliente HTTP/WS  →  API Go  →  Copilot SDK (JSON-RPC)  →  Copilot CLI  →  GitHub Copilot
```

## Endpoints

### Endpoints nativos

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/health` | Health check |
| `GET` | `/models` | Lista modelos disponíveis classificados por tier |
| `POST` | `/chat` | Chat síncrono — JSON ou `multipart/form-data` |
| `WS` | `/chat/stream` | Streaming de tokens via WebSocket |

### Endpoints OpenAI-compatible

| Método | Rota | Descrição |
|--------|------|-----------|
| `GET` | `/v1/models` | Lista modelos (formato OpenAI) |
| `POST` | `/v1/chat/completions` | Chat síncrono ou SSE streaming (formato OpenAI) |

## Quick Start

### Local

```bash
export COPILOT_GITHUB_TOKEN=github_pat_xxxxx
go mod download
go build -o api-server .
./api-server
```

### Docker

```bash
echo "COPILOT_GITHUB_TOKEN=github_pat_xxxxx" > .env
docker compose up --build
```

## Exemplos de uso

<details>
<summary><strong>Health check</strong></summary>

```bash
curl http://localhost:8080/health
```
</details>

<details>
<summary><strong>Listar modelos</strong></summary>

```bash
curl http://localhost:8080/models | jq
```
</details>

<details>
<summary><strong>Chat (JSON)</strong></summary>

```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"prompt":"Explique ponteiros em Go","model":"gpt-5-mini"}'
```
</details>

<details>
<summary><strong>Chat com arquivo</strong></summary>

```bash
curl -X POST http://localhost:8080/chat \
  -F "prompt=Descreva esta imagem" \
  -F "model=gpt-4o" \
  -F "file=@screenshot.png"
```
</details>

<details>
<summary><strong>Streaming via WebSocket</strong></summary>

```javascript
const ws = new WebSocket("ws://localhost:8080/chat/stream");

ws.onopen = () => {
  ws.send(JSON.stringify({
    prompt: "Escreva um hello world em Go",
    model: "gpt-5-mini"
  }));
};

ws.onmessage = (e) => process.stdout.write(e.data);
```
</details>

## OpenAI-Compatible (Drop-in Replacement)

Os endpoints `/v1/...` permitem usar esta API como **drop-in replacement** de uma API OpenAI. Compatível com:

- **OpenClaude** / **Claude Code** (`OPENAI_BASE_URL=http://localhost:8080/v1`)
- **Aider**, **Continue**, **Open WebUI**, **LiteLLM**
- Qualquer client que fale o protocolo OpenAI

<details>
<summary><strong>Exemplo: Listar modelos (formato OpenAI)</strong></summary>

```bash
curl http://localhost:8080/v1/models | jq '.data[].id'
```
</details>

<details>
<summary><strong>Exemplo: Chat síncrono (formato OpenAI)</strong></summary>

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dummy" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello"}]
  }' | jq '.choices[0].message.content'
```
</details>

<details>
<summary><strong>Exemplo: Streaming SSE (formato OpenAI)</strong></summary>

```bash
curl -N -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello world em Go"}],
    "stream": true
  }'
```
</details>

<details>
<summary><strong>Exemplo: Usar com OpenClaude / Claude Code</strong></summary>

```bash
export CLAUDE_CODE_USE_OPENAI=1
export OPENAI_BASE_URL=http://localhost:8080/v1
export OPENAI_API_KEY=dummy
export OPENAI_MODEL=gpt-4o
openclaude
```
</details>

## Arquitetura

| Componente | Tecnologia |
|-----------|------------|
| HTTP server | `net/http` nativo (Go 1.22+ ServeMux) |
| WebSocket | `gorilla/websocket` |
| SDK client | `github.com/github/copilot-sdk/go` v0.1.29 |
| Container | `debian:bookworm-slim` (multi-stage build) |

### Como funciona

- Cada request cria uma **sessão efêmera** no Copilot SDK
- O lifecycle do SDK fica centralizado em um **Manager** thread-safe
- Uploads são salvos em `/tmp/copilot-uploads` (isolado do CLI), usados na sessão e apagados automaticamente
- Modelos são classificados por **match exato + fallback por prefixo** (IDs versionados como `gpt-4o-2024-08-06` → `gpt-4o`)

### Upload de arquivos

| Canal | Formato | Campo |
|-------|---------|-------|
| `POST /chat` | JSON ou multipart | `file` (opcional) |
| `WS /chat/stream` | JSON | `file.data` em base64 (opcional) |

- **Limite:** 5 MB
- **Extensões:** imagens, texto, código, PDF, configs e logs

## Notas técnicas

<details>
<summary><strong>Consumo de memória (~250 MiB)</strong></summary>

O alto uso de memória inicial **não** vem da API Go:

| Processo | RSS |
|----------|-----|
| API Go (`/api-server`) | ~10 MiB |
| Copilot CLI embutido | ~295 MiB |
| **Container total** | **~216–245 MiB** |

A maior parte da memória é do **Copilot CLI** (runtime Node.js embutido, ~133 MiB em disco).
</details>

<details>
<summary><strong>Sobre o endpoint /models</strong></summary>

`/models` retorna apenas o que o Copilot CLI anuncia via `ListModels()`. Um modelo pode funcionar no `/chat` e não aparecer em `/models` — depende do que o CLI expõe para a conta/ambiente atual.
</details>

<details>
<summary><strong>Healthcheck do container</strong></summary>

O healthcheck usa `curl` em vez de `/dev/tcp` porque o `/bin/sh` do Debian slim é `dash`, que não suporta essa sintaxe bash.
</details>

## Requisitos

- **Go** 1.25+
- **Docker** + Docker Compose (opcional)
- **Token** `github_pat_...` com permissão **Copilot Requests**

## Estrutura do projeto

```
.
├── main.go              # Entry point, rotas e graceful shutdown
├── go.mod
├── copilot/
│   ├── client.go        # Manager do SDK (lifecycle, token, mutex)
│   └── models.go        # Classificação de modelos por tier
├── handlers/
│   ├── chat.go          # POST /chat
│   ├── models.go        # GET /models
│   ├── openai.go        # POST /v1/chat/completions (sync + SSE)
│   ├── openai_models.go # GET /v1/models
│   ├── openai_types.go  # Structs request/response OpenAI
│   ├── response.go      # Helpers de resposta JSON
│   ├── stream.go        # WS /chat/stream
│   └── upload.go        # Validação e gestão de uploads
├── middleware/
│   └── middleware.go     # Recoverer, CORS, Logger
├── Dockerfile
├── docker-compose.yml
└── README.md
```

---

# 🇺🇸 English

## Overview

HTTP API and WebSocket server wrapping the [GitHub Copilot SDK](https://github.com/github/copilot-sdk) for synchronous chat, token streaming, and file analysis.

```
HTTP/WS Client  →  Go API  →  Copilot SDK (JSON-RPC)  →  Copilot CLI  →  GitHub Copilot
```

## Endpoints

### Native endpoints

| Method | Route | Description |
|--------|-------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/models` | Lists available models classified by tier |
| `POST` | `/chat` | Synchronous chat — JSON or `multipart/form-data` |
| `WS` | `/chat/stream` | Token streaming over WebSocket |

### OpenAI-compatible endpoints

| Method | Route | Description |
|--------|-------|-------------|
| `GET` | `/v1/models` | List models (OpenAI format) |
| `POST` | `/v1/chat/completions` | Sync or SSE streaming chat (OpenAI format) |

## Quick Start

### Local

```bash
export COPILOT_GITHUB_TOKEN=github_pat_xxxxx
go mod download
go build -o api-server .
./api-server
```

### Docker

```bash
echo "COPILOT_GITHUB_TOKEN=github_pat_xxxxx" > .env
docker compose up --build
```

## Usage examples

<details>
<summary><strong>Health check</strong></summary>

```bash
curl http://localhost:8080/health
```
</details>

<details>
<summary><strong>List models</strong></summary>

```bash
curl http://localhost:8080/models | jq
```
</details>

<details>
<summary><strong>Chat (JSON)</strong></summary>

```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"prompt":"Explain pointers in Go","model":"gpt-5-mini"}'
```
</details>

<details>
<summary><strong>Chat with file</strong></summary>

```bash
curl -X POST http://localhost:8080/chat \
  -F "prompt=Describe this image" \
  -F "model=gpt-4o" \
  -F "file=@screenshot.png"
```
</details>

<details>
<summary><strong>WebSocket streaming</strong></summary>

```javascript
const ws = new WebSocket("ws://localhost:8080/chat/stream");

ws.onopen = () => {
  ws.send(JSON.stringify({
    prompt: "Write a hello world in Go",
    model: "gpt-5-mini"
  }));
};

ws.onmessage = (e) => process.stdout.write(e.data);
```
</details>

## OpenAI-Compatible (Drop-in Replacement)

The `/v1/...` endpoints allow using this API as a **drop-in replacement** for an OpenAI API. Compatible with:

- **OpenClaude** / **Claude Code** (`OPENAI_BASE_URL=http://localhost:8080/v1`)
- **Aider**, **Continue**, **Open WebUI**, **LiteLLM**
- Any client that speaks the OpenAI protocol

<details>
<summary><strong>List models (OpenAI format)</strong></summary>

```bash
curl http://localhost:8080/v1/models | jq '.data[].id'
```
</details>

<details>
<summary><strong>Sync chat (OpenAI format)</strong></summary>

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dummy" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello"}]
  }' | jq '.choices[0].message.content'
```
</details>

<details>
<summary><strong>SSE streaming (OpenAI format)</strong></summary>

```bash
curl -N -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello world in Go"}],
    "stream": true
  }'
```
</details>

<details>
<summary><strong>Use with OpenClaude / Claude Code</strong></summary>

```bash
export CLAUDE_CODE_USE_OPENAI=1
export OPENAI_BASE_URL=http://localhost:8080/v1
export OPENAI_API_KEY=dummy
export OPENAI_MODEL=gpt-4o
openclaude
```
</details>

## Architecture

| Component | Technology |
|-----------|------------|
| HTTP server | Native `net/http` (Go 1.22+ ServeMux) |
| WebSocket | `gorilla/websocket` |
| SDK client | `github.com/github/copilot-sdk/go` v0.1.29 |
| Container | `debian:bookworm-slim` (multi-stage build) |

### How it works

- Each request creates an **ephemeral session** in the Copilot SDK
- SDK lifecycle is centralized in a **thread-safe Manager**
- Uploaded files are saved to `/tmp/copilot-uploads` (isolated from the CLI), used in the session, and automatically cleaned up
- Models are classified by **exact match + prefix fallback** (versioned IDs like `gpt-4o-2024-08-06` → `gpt-4o`)

### File upload

| Channel | Format | Field |
|---------|--------|-------|
| `POST /chat` | JSON or multipart | `file` (optional) |
| `WS /chat/stream` | JSON | `file.data` as base64 (optional) |

- **Limit:** 5 MB
- **Extensions:** images, text, code, PDF, config and log files

## Technical notes

<details>
<summary><strong>Memory usage (~250 MiB)</strong></summary>

The high initial memory usage does **not** come from the Go API:

| Process | RSS |
|---------|-----|
| Go API (`/api-server`) | ~10 MiB |
| Embedded Copilot CLI | ~295 MiB |
| **Container total** | **~216–245 MiB** |

Most memory is consumed by the **Copilot CLI** (embedded Node.js runtime, ~133 MiB on disk).
</details>

<details>
<summary><strong>About the /models endpoint</strong></summary>

`/models` only returns what the Copilot CLI advertises via `ListModels()`. A model may work in `/chat` but not appear in `/models` — it depends on what the CLI exposes for the current account/environment.
</details>

<details>
<summary><strong>Container healthcheck</strong></summary>

The healthcheck uses `curl` instead of `/dev/tcp` because Debian slim's `/bin/sh` is `dash`, which doesn't support that bash syntax.
</details>

## Requirements

- **Go** 1.25+
- **Docker** + Docker Compose (optional)
- **Token** `github_pat_...` with **Copilot Requests** permission

## Project structure

```
.
├── main.go              # Entry point, routes and graceful shutdown
├── go.mod
├── copilot/
│   ├── client.go        # SDK Manager (lifecycle, token, mutex)
│   └── models.go        # Model tier classification
├── handlers/
│   ├── chat.go          # POST /chat
│   ├── models.go        # GET /models
│   ├── response.go      # JSON response helpers
│   ├── stream.go        # WS /chat/stream
│   └── upload.go        # Upload validation and management
├── middleware/
│   └── middleware.go     # Recoverer, CORS, Logger
├── Dockerfile
├── docker-compose.yml
└── README.md
```

---

<div align="center">

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=J3rendow/copilot-api&type=Date)](https://www.star-history.com/#J3rendow/copilot-api&Date)

</div>

### Example requests

```bash
curl http://localhost:8080/health
curl http://localhost:8080/models | jq
```

```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"prompt":"Explain Go pointers in one sentence","model":"gpt-5-mini"}'
```

```bash
curl -X POST http://localhost:8080/chat \
  -F "prompt=Describe this image" \
  -F "model=gpt-4o" \
  -F "file=@screenshot.png"
```

### Repository growth

If the repository gains traction, the badges and the Star History chart at the top of this README will update automatically.

### License

Internal use / local development.
| `WS /chat/stream` | JSON com `file.data` base64 | Base64 decodificado → arquivo temp → `Attachment{Type: "file", Path: tmpPath}` |

**Segurança:**
- Extensão validada contra allowlist (evita upload de executáveis)
- Tamanho limitado a 5MB (validado no handler e no `saveTempFile`)
- `filepath.Base()` aplicado ao nome do arquivo (previne path traversal)
- Arquivo temporário removido via `defer cleanup()` após uso
- Diretório temporário isolado por request (`os.MkdirTemp`)

### Classificação de Modelos

A classificação usa **duas estratégias de matching** para mapear model IDs aos multiplicadores oficiais:

1. **Match exato** — `gpt-4o` → encontrado diretamente no mapa
2. **Prefix match** (fallback) — `gpt-4o-2024-08-06` → prefixo `gpt-4o` encontrado  
   Prefixos são testados do mais longo ao mais curto para evitar ambiguidade.

Modelos incluídos (0× — não consomem premium requests):
- `gpt-4.1`, `gpt-4o`, `gpt-5-mini`, `raptor-mini`

Todos os demais são classificados como `premium_request` com multiplicador conforme tabela.  
Modelos desconhecidos (não mapeados) recebem multiplicador conservador de 1×.

### Docker — Empacotamento do CLI

O SDK utiliza o executável do Copilot CLI internamente. O CLI é um binário Node.js autocontido (~132MB) que requer **glibc + libstdc++**.

**Build multi-stage:**

```
golang:bookworm (builder)
  → go tool bundler      (embute CLI no binário Go via embed)
  → go build             (compila binário estático)
  → pré-extrai CLI       (para startup mais rápido)

debian:bookworm-slim (runtime, ~80MB)
  → binário Go + CLI pré-extraído
  → ca-certificates
  → usuário não-root (appuser)
  → HEALTHCHECK nativo
```

> **Por que não Alpine/distroless?** O CLI do Copilot é linkado contra glibc.
> Alpine usa musl (incompatível) e distroless/static não tem nenhuma lib dinâmica.

### Por que a API usa ~250 MiB ao iniciar?

Essa aplicação não inicia apenas um binário Go. Ela inicia **dois processos**:

1. `api-server` em Go, responsável por HTTP, WebSocket, parse de payloads, timeouts e middleware
2. `copilot` CLI, responsável por toda a comunicação agentic com o backend do GitHub Copilot

Na prática:

- O processo Go ficou em torno de `10MiB` de RSS
- O processo `copilot` ficou em torno de `295MiB` de RSS
- O `docker stats` mostrou o container em ~`244.6MiB`

Os números não batem exatamente porque:

- `docker top` mostra RSS por processo
- `docker stats` mostra memória pelo cgroup do container
- páginas compartilhadas, page cache e diferenças de amostragem afetam a comparação

Mas a direção é inequívoca: **quase toda a memória está no Copilot CLI**, não no código Go da API.

Implicação prática:

- trocar Go por Rust **não** elimina o processo do Copilot CLI
- portanto, **não** se deve esperar uma redução drástica de memória total apenas pela troca de linguagem da API

## Análise de Viabilidade — Versão em Rust

Objetivo analisado: manter a versão atual em Go e, se fizer sentido, adicionar uma implementação paralela em Rust em uma subpasta do mesmo repositório, com Docker/Compose próprios, sem apagar a base existente.

### O que existe hoje no ecossistema Rust

Foi validado o repositório comunitário [copilot-community-sdk/copilot-sdk-rust](https://github.com/copilot-community-sdk/copilot-sdk-rust):

- é **community-maintained**, não oficial
- está em **technical preview**
- declara **paridade ampla** com os SDKs oficiais
- suporta sessões, streaming, tools, hooks, shell ops, MCP, BYOK e protocol v2/v3
- **não possui CLI bundling** pronto (`CLI bundling: planned`)
- não há releases publicadas no momento
- a base é recente e com comunidade pequena

Também foi validado o org [copilot-community-sdk](https://github.com/copilot-community-sdk), que explicita o disclaimer de que esses SDKs são **não oficiais** e podem quebrar com mudanças no Copilot CLI.

### Impacto esperado em performance

Trocar a camada HTTP/WebSocket de Go para Rust pode trazer ganhos marginais em:

- uso de CPU sob alta concorrência
- latência do servidor para parse/serialização
- alocação de memória da própria camada de API
- previsibilidade em cargas muito altas

Mas, neste projeto específico, o gargalo principal não está nessa camada. O fluxo dominante é:

`HTTP/WS -> API -> SDK -> Copilot CLI -> GitHub Copilot`

Ou seja:

- o custo maior está no processo do Copilot CLI
- a latência principal vem do processamento remoto + CLI intermediário
- o ganho da troca Go -> Rust tende a ser **secundário** no resultado final

### Impacto esperado em memória

Cenário atual observado:

- Go API: ~`10MiB`
- Copilot CLI: ~`295MiB` RSS

Mesmo que uma versão Rust reduzisse a API de ~`10MiB` para ~`5MiB` ou menos, o total do container continuaria dominado pelo CLI. Na prática:

- economia absoluta provável: **pequena**
- economia percentual total: **baixa**
- benefício real de memória: **insuficiente** para justificar sozinho uma reescrita

### Riscos da alternativa Rust

1. SDK não oficial
2. menor maturidade operacional que o SDK oficial em Go
3. risco de divergência futura com novas features do protocolo
4. maior custo de manutenção em paralelo entre duas bases
5. ausência atual de bundling do CLI, o que pode complicar a distribuição comparado ao fluxo atual em Go

### Quando vale a pena fazer a versão Rust

Vale a pena se o objetivo for:

- benchmark comparativo formal entre stacks
- explorar ecossistema Rust para runtime, observabilidade ou integração futura
- ter uma segunda implementação de referência
- reduzir overhead da camada HTTP sob alta escala

Não vale a pena se o objetivo principal for apenas:

- reduzir de forma relevante a memória total do container
- obter grande melhora de latência ponta a ponta

### Recomendação

Recomendação técnica atual:

- **manter Go como implementação principal**
- **não migrar por causa de memória**
- se houver interesse em benchmark, criar uma versão paralela em Rust em subpasta separada, sem remover nada da versão Go

Estrutura sugerida para essa evolução futura:

```text
.
├── go-api/                  # versão atual (ou manter raiz como está)
├── rust-api/
│   ├── Cargo.toml
│   ├── src/
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── README.md
└── benchmarks/
  └── cenarios.md
```

Critério de decisão recomendado antes de implementar tudo em Rust:

1. medir throughput com N requests concorrentes
2. medir p50/p95/p99 em `/chat` e `/chat/stream`
3. medir memória total do container em idle e sob carga
4. verificar custo operacional do bundling/distribuição do CLI no stack Rust
5. só continuar se houver ganho material comprovado

### Middleware Chain

```
Request → Recoverer → CORS → Logger → Router → Handler → Response
```

- **Recoverer**: captura panics e retorna `500` (evita crash do processo)
- **CORS**: headers permissivos para desenvolvimento (`Access-Control-Allow-Origin: *`)
- **Logger**: log estruturado JSON via `slog` (método, path, status, duração, remote)

### Timeouts do Servidor

| Timeout | Valor | Motivo |
|---------|-------|--------|
| `ReadHeaderTimeout` | 10s | Protege contra slowloris |
| `ReadTimeout` | 30s | Suficiente para upload de 5MB |
| `WriteTimeout` | 10min | Streaming pode demorar (modelos lentos) |
| `IdleTimeout` | 120s | Keep-alive entre requests |
| Context (chat) | 5min | `context.WithTimeout` independente do `ReadTimeout` |
| Context (stream) | 5min | Timeout total para toda a sessão de streaming |

### Permissões do SDK

Todas as sessões usam `sdk.PermissionHandler.ApproveAll` para permitir que o agente utilize ferramentas (leitura de arquivos, web requests, etc.) sem interação manual.

## Variáveis de Ambiente

| Variável | Descrição | Default |
|----------|-----------|---------|
| `PORT` | Porta do servidor HTTP | `8080` |
| `COPILOT_GITHUB_TOKEN` | Token de autenticação do Copilot (prioridade 1) | — |
| `GH_TOKEN` | Token alternativo (prioridade 2) | — |
| `GITHUB_TOKEN` | Token alternativo (prioridade 3) | — |

## Limites de Recursos (Docker)

| Recurso | Limite | Reserva |
|---------|--------|---------|
| Memória | 1024MB | 256MB |
| CPU | 1.0 | 0.25 |

## Licença

Uso interno / desenvolvimento local.
