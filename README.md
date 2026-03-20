# Copilot API

[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=for-the-badge&logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=for-the-badge&logo=docker&logoColor=white)](https://www.docker.com/)
[![GitHub stars](https://img.shields.io/github/stars/J3rendow/copilot-api?style=for-the-badge)](https://github.com/J3rendow/copilot-api/stargazers)
[![GitHub forks](https://img.shields.io/github/forks/J3rendow/copilot-api?style=for-the-badge)](https://github.com/J3rendow/copilot-api/network/members)
[![Last commit](https://img.shields.io/github/last-commit/J3rendow/copilot-api?style=for-the-badge)](https://github.com/J3rendow/copilot-api/commits/main)

High-performance REST API + WebSocket server built in Go on top of the official GitHub Copilot SDK.

API REST + servidor WebSocket em Go, usando o SDK oficial do GitHub Copilot para chat síncrono, streaming e análise de arquivos.

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=J3rendow/copilot-api&type=Date)](https://www.star-history.com/#J3rendow/copilot-api&Date)

## Languages

- [Português](#português)
- [English](#english)

## Português

### Visão Geral

Esta aplicação expõe uma API HTTP e um endpoint WebSocket sobre o GitHub Copilot SDK (`github.com/github/copilot-sdk/go`).

Fluxo real da aplicação:

1. O cliente envia requisição para a API Go
2. A API cria uma sessão efêmera no SDK
3. O SDK fala com o Copilot CLI via JSON-RPC
4. O Copilot CLI se comunica com o backend do GitHub Copilot
5. A resposta volta para a API e é entregue por HTTP ou WebSocket

Arquitetura resumida:

```text
Cliente HTTP/WS
  -> API Go
  -> SDK oficial do Copilot
  -> Copilot CLI
  -> GitHub Copilot
```

### Endpoints

| Método | Path | Descrição |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/models` | Lista os modelos retornados pelo CLI e classifica por tier |
| POST | `/chat` | Chat síncrono com JSON ou multipart/form-data |
| WS | `/chat/stream` | Streaming de resposta via WebSocket |

### Como funciona hoje

- O servidor HTTP usa `net/http` nativo
- O streaming usa `gorilla/websocket`
- O lifecycle do SDK fica centralizado no manager
- Cada request cria sua própria sessão efêmera no Copilot SDK
- Uploads são temporários e apagados após o uso
- O container roda com `debian:bookworm-slim`

### Consumo de memória no startup

O uso de memória inicial mais alto não vem do servidor Go em si.

Validação em runtime:

- `docker stats`: cerca de `216 MiB` a `245 MiB`
- `/api-server`: cerca de `10 MiB` de RSS
- Copilot CLI: cerca de `295 MiB` de RSS em `docker top`
- cache extraído do CLI: cerca de `133 MiB` em disco

Conclusão:

- o processo pesado é o **Copilot CLI embutido**
- a API Go é relativamente leve
- a maior parte da memória do container é dominada pelo runtime do CLI

### Importante sobre `/models`

O endpoint `/models` mostra apenas o que o Copilot CLI retorna em `ListModels()`.

Então:

- um modelo pode funcionar no `/chat`
- e ainda assim não aparecer em `/models`

Isso acontece porque a listagem depende do que o CLI anuncia para a conta/ambiente naquele momento.

### Modelos e classificação

- a API usa classificação por match exato e fallback por prefixo
- isso permite mapear IDs versionados como `gpt-4o-2024-08-06` para `gpt-4o`
- modelos desconhecidos recebem multiplicador conservador `1x`

### Upload de arquivos

`POST /chat`:

- aceita `application/json`
- aceita `multipart/form-data`
- campo opcional `file`

`WS /chat/stream`:

- aceita JSON com `file.data` em base64
- o arquivo é decodificado e salvo temporariamente antes de ser enviado ao SDK

Limites:

- tamanho máximo: `5 MB`
- extensões permitidas: imagens, texto, código, PDF e alguns formatos de configuração/log

### Quick Start

#### Local

```bash
export COPILOT_GITHUB_TOKEN=github_pat_xxxxx
go mod download
go build -o api-server .
./api-server
```

#### Docker

```bash
echo "COPILOT_GITHUB_TOKEN=github_pat_xxxxx" > .env
docker compose up --build
```

### Exemplos

#### Health

```bash
curl http://localhost:8080/health
```

#### Models

```bash
curl http://localhost:8080/models | jq
```

#### Chat JSON

```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{"prompt":"Explique ponteiros em Go","model":"gpt-5-mini"}'
```

#### Chat com arquivo

```bash
curl -X POST http://localhost:8080/chat \
  -F "prompt=Descreva esta imagem" \
  -F "model=gpt-4o" \
  -F "file=@screenshot.png"
```

#### Streaming WebSocket

```javascript
const ws = new WebSocket("ws://localhost:8080/chat/stream");

ws.onopen = () => {
  ws.send(JSON.stringify({
    prompt: "Escreva um hello world em Go",
    model: "gpt-5-mini"
  }));
};
```

### Estrutura do projeto

```text
.
├── main.go
├── go.mod
├── copilot/
│   ├── client.go
│   └── models.go
├── handlers/
│   ├── chat.go
│   ├── models.go
│   ├── response.go
│   ├── stream.go
│   └── upload.go
├── middleware/
│   └── middleware.go
├── Dockerfile
├── docker-compose.yml
└── README.md
```

### Requisitos

- Go `1.25+`
- Docker e Docker Compose opcionalmente
- token `github_pat_...` com permissão `Copilot Requests`

### Healthcheck do container

O runtime usa `curl` no healthcheck porque `/dev/tcp` não é compatível com o `/bin/sh` do Debian slim.

---

## English

### Overview

This project exposes an HTTP API and a WebSocket endpoint on top of the official GitHub Copilot Go SDK.

Actual request flow:

1. Client sends a request to the Go API
2. The API creates an ephemeral SDK session
3. The SDK talks to the Copilot CLI over JSON-RPC
4. The Copilot CLI talks to GitHub Copilot
5. The response comes back through HTTP or WebSocket

### Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/models` | Lists CLI-exposed models and classifies them by tier |
| POST | `/chat` | Synchronous chat with JSON or multipart/form-data |
| WS | `/chat/stream` | Streaming responses over WebSocket |

### Runtime notes

- HTTP server uses native `net/http`
- WebSocket streaming uses `gorilla/websocket`
- SDK lifecycle is centralized in a manager
- each chat request creates its own temporary Copilot session
- uploaded files are temporary and cleaned up after use

### Why memory usage starts around 200+ MiB

The Go server is not the main memory consumer.

Observed runtime profile:

- container memory: roughly `216 MiB` to `245 MiB`
- `/api-server`: about `10 MiB` RSS
- embedded Copilot CLI: roughly `295 MiB` RSS in `docker top`

Bottom line:

- most memory is consumed by the embedded Copilot CLI process
- the Go API layer itself is comparatively small

### Important note about `/models`

`/models` only returns what the Copilot CLI exposes through `ListModels()`.

So a model may:

- work in `/chat`
- but still not appear in `/models`

if the CLI accepts the model ID directly but does not advertise it in the current account/environment listing.

### File support

`POST /chat` supports:

- `application/json`
- `multipart/form-data`
- optional `file` upload

`WS /chat/stream` supports:

- JSON payloads
- optional `file.data` in base64

Limits:

- max file size: `5 MB`
- allowed file extensions for images, text, code, PDF and selected config/log formats

### Quick Start

```bash
export COPILOT_GITHUB_TOKEN=github_pat_xxxxx
go mod download
go build -o api-server .
./api-server
```

```bash
echo "COPILOT_GITHUB_TOKEN=github_pat_xxxxx" > .env
docker compose up --build
```

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
