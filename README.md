# API REST + WebSocket — GitHub Copilot SDK (Go)

API altamente performática para automação local usando o GitHub Copilot SDK (`github.com/github/copilot-sdk/go` v0.1.29).  
O SDK funciona como cliente JSON-RPC que se comunica com o executável do Copilot CLI em "server mode".

## Arquitetura

```
┌──────────────┐     HTTP/WS      ┌──────────────────┐   JSON-RPC   ┌──────────────┐
│  Cliente     │ ◄──────────────► │  API Go (8080)   │ ◄──────────► │  Copilot CLI │
│  (curl/app)  │                  │  net/http + WS   │              │  (server)    │
└──────────────┘                  └──────────────────┘              └──────────────┘
```

**Fluxo interno:**
1. O cliente HTTP/WS envia um request para a API Go
2. A API cria uma sessão efêmera no SDK (`CreateSession`)
3. O SDK traduz para JSON-RPC e envia ao Copilot CLI
4. O CLI comunica com a API do GitHub Copilot (cloud)
5. A resposta retorna pelo mesmo caminho

## Validação da Arquitetura Atual

Arquitetura validada em runtime:

1. O processo principal da aplicação é o binário Go `/api-server`
2. Na inicialização, `mgr.Start(ctx)` sobe o cliente do SDK e mantém o Copilot CLI em execução contínua
3. O Copilot CLI roda como um segundo processo dentro do mesmo container, em modo `--headless --stdio`
4. O servidor HTTP usa `net/http` nativo com `ServeMux`, sem framework adicional
5. O WebSocket usa `gorilla/websocket` apenas no endpoint de streaming
6. Cada request de chat cria uma sessão efêmera do SDK, envia a mensagem e destrói a sessão ao final
7. Uploads são temporários e descartados após a resposta

Medições observadas no container em execução:

- `docker stats`: `244.6MiB / 1GiB`
- `docker top`: `/api-server` com RSS de ~`10MiB`
- `docker top`: Copilot CLI com RSS de ~`295MiB`
- Cache extraído do CLI em disco: ~`133MiB` em `/home/appuser/.cache/copilot-sdk`

Conclusão objetiva:

- O consumo de memória não é do servidor Go
- O consumo dominante vem do **Copilot CLI embutido**, que é um runtime Node.js autocontido
- A API Go adiciona pouca memória incremental; o peso está no processo auxiliar obrigatório do SDK


## Endpoints

| Método | Path           | Content-Type                        | Descrição                                                |
|--------|----------------|-------------------------------------|----------------------------------------------------------|
| GET    | `/health`      | —                                   | Health check (`{"status":"ok"}`)                         |
| GET    | `/models`      | —                                   | Lista modelos classificados (free vs premium) com multiplicadores |
| POST   | `/chat`        | `application/json` ou `multipart/form-data` | Chat síncrono — aceita texto e/ou arquivo (≤5MB) |
| WS     | `/chat/stream` | JSON via WebSocket                  | Streaming de tokens — aceita anexo base64 (≤5MB)         |

## Estrutura do Projeto

```
.
├── main.go                 # Entry point, roteamento, graceful shutdown
├── go.mod                  # Módulo Go (requer Go 1.25+)
├── copilot/
│   ├── client.go           # Manager do ciclo de vida do SDK (token, start/stop)
│   └── models.go           # Classificação de modelos com multiplicadores oficiais
├── handlers/
│   ├── response.go         # Helpers de resposta JSON (JSON, JSONError)
│   ├── upload.go           # Validação de uploads, temp files, builders de Attachment
│   ├── models.go           # GET /models (listagem dinâmica via SDK)
│   ├── chat.go             # POST /chat (JSON + multipart/form-data com arquivo)
│   └── stream.go           # WS /chat/stream (streaming + anexo base64)
├── middleware/
│   └── middleware.go        # Logger (slog), Recoverer (panic → 500), CORS
├── Dockerfile              # Multi-stage: golang:bookworm → debian:bookworm-slim
├── docker-compose.yml      # Orquestração com limite de recursos (1024MB, 1 CPU)
├── .env                    # Token de autenticação (NÃO commitar)
└── .gitignore
```

## Pré-requisitos

- **Go 1.25+** (usa `net/http` ServeMux com method patterns + go tool)
- **Fine-grained PAT** (`github_pat_...`) com permissão **"Copilot Requests"**  
  ⚠️ Classic PATs (`ghp_...`) **NÃO** são suportados pelo Copilot CLI
- **Docker + Docker Compose** (opcional, para execução containerizada)

## Autenticação

A API autentica com o GitHub Copilot via token. O SDK verifica as variáveis na ordem:

1. `COPILOT_GITHUB_TOKEN`
2. `GH_TOKEN`
3. `GITHUB_TOKEN`

**Tipos de token suportados:**

| Tipo | Prefixo | Suportado | Notas |
|------|---------|-----------|-------|
| Fine-grained PAT | `github_pat_` | ✅ | Requer permissão "Copilot Requests" |
| OAuth token | `gho_` | ✅ | Via `copilot auth login` ou `gh auth login` |
| User-to-server | `ghu_` | ✅ | Via GitHub App |
| Classic PAT | `ghp_` | ❌ | **Rejeitado** pelo CLI — log de erro na inicialização |

## Início Rápido (Local)

```bash
# 1. Clone e entre no diretório
cd api-rest-copilot

# 2. Configure o token (fine-grained PAT com permissão "Copilot Requests")
export COPILOT_GITHUB_TOKEN=github_pat_xxxxx

# 3. Instale dependências e compile
go mod download
go build -o api-server .

# 4. Execute
./api-server
# {"level":"INFO","msg":"main: servidor iniciado","port":"8080"}
```

## Início Rápido (Docker)

```bash
# 1. Configure o token
echo "COPILOT_GITHUB_TOKEN=github_pat_xxxxx" > .env

# 2. Build e execução
docker compose up --build

# Imagem final: debian:bookworm-slim (~80MB)
# Runtime: golang:bookworm (builder) → debian:bookworm-slim (produção)
# O Copilot CLI (~132MB Node.js) é embutido no binário via bundler do SDK.
```

## Exemplos de Uso

---

### GET /health

```bash
curl http://localhost:8080/health
```
```json
{"status":"ok"}
```

---

### GET /models

Lista todos os modelos disponíveis classificados por tier com seus multiplicadores de custo.

```bash
curl http://localhost:8080/models | jq
```
```json
{
  "free": [
    {"id": "gpt-4.1", "name": "gpt-4.1", "tier": "free_0x", "multiplier": 0},
    {"id": "gpt-4o", "name": "gpt-4o", "tier": "free_0x", "multiplier": 0},
    {"id": "gpt-5-mini", "name": "gpt-5-mini", "tier": "free_0x", "multiplier": 0}
  ],
  "premium": [
    {"id": "claude-sonnet-4", "name": "claude-sonnet-4", "tier": "premium_request", "multiplier": 1},
    {"id": "gemini-2.5-pro", "name": "gemini-2.5-pro", "tier": "premium_request", "multiplier": 1},
    {"id": "gpt-5.1", "name": "gpt-5.1", "tier": "premium_request", "multiplier": 1}
  ],
  "total": 17
}
```

> **Nota:** Os modelos disponíveis dependem do seu plano GitHub Copilot (Free, Pro, Pro+, Business, Enterprise).  
> A classificação usa **prefix matching** — modelos versionados como `gpt-4o-2024-08-06` são corretamente mapeados para `gpt-4o` (free_0x).

> **Comportamento importante:** o endpoint `/models` mostra apenas os modelos que o Copilot CLI anuncia em `ListModels()`.  
> Isso significa que um modelo pode funcionar em `POST /chat` mesmo sem aparecer em `/models`, caso o CLI aceite esse ID diretamente, mas não o exponha na listagem atual da conta/ambiente. Em uma validação recente, o CLI retornou `gpt-4.1` e `gpt-5-mini`, mas não retornou `gpt-4o` na lista, embora `gpt-4o` tenha respondido normalmente no endpoint `/chat`.

**Multiplicadores de custo (planos pagos):**

| Modelo | Multiplicador | Tier |
|--------|--------------|------|
| `gpt-4.1`, `gpt-4o`, `gpt-5-mini`, `raptor-mini` | 0× | free_0x |
| `claude-haiku-4.5`, `gemini-3-flash`, `gpt-5.1-codex-mini`, `grok-code-fast-1` | 0.25–0.33× | premium |
| `claude-sonnet-4`, `claude-sonnet-4.5`, `gemini-2.5-pro`, `gpt-5.1`, `gpt-5.2` | 1× | premium |
| `claude-opus-4.5`, `claude-opus-4.6` | 3× | premium |
| `claude-opus-4.6-fast` | 30× | premium |

---

### POST /chat (JSON — texto puro)

Chat síncrono: envia um prompt e aguarda a resposta completa.

```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Explique ponteiros em Go em uma frase.",
    "model": "gpt-5-mini"
  }'
```
```json
{
  "model": "gpt-5-mini",
  "content": "Ponteiros em Go são variáveis que armazenam o endereço de memória de outra variável, permitindo acesso e modificação indireta do valor original."
}
```

**Campos do request (JSON):**

| Campo | Tipo | Obrigatório | Default | Descrição |
|-------|------|-------------|---------|-----------|
| `prompt` | string | ✅ | — | Prompt/pergunta para o modelo |
| `model` | string | ❌ | `gpt-5-mini` | ID do modelo (ver `/models`) |
| `reasoning_effort` | string | ❌ | — | Nível de raciocínio: `low`, `medium`, `high`, `xhigh` |

---

### POST /chat (JSON — com reasoning)

Modelos que suportam "thinking" retornam o raciocínio interno no campo `reasoning`.

```bash
curl -X POST http://localhost:8080/chat \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "Qual o 20º número primo?",
    "model": "claude-sonnet-4",
    "reasoning_effort": "high"
  }'
```
```json
{
  "model": "claude-sonnet-4",
  "content": "O 20º número primo é 71.",
  "reasoning": "Vou listar os números primos sequencialmente: 2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41, 43, 47, 53, 59, 61, 67, 71. O 20º é 71."
}
```

---

### POST /chat (Multipart — com arquivo)

Envio de arquivo (imagem, código, texto, PDF) junto com o prompt via `multipart/form-data`.  
O arquivo é salvo temporariamente no servidor e passado ao SDK como `Attachment{Type: "file"}`.

```bash
# Transcrever/analisar uma imagem
curl -X POST http://localhost:8080/chat \
  -F "prompt=Descreva o que você vê nesta imagem em detalhes" \
  -F "model=gpt-4o" \
  -F "file=@screenshot.png"

# Analisar código Go
curl -X POST http://localhost:8080/chat \
  -F "prompt=Encontre bugs e sugira melhorias neste código" \
  -F "model=gpt-5-mini" \
  -F "file=@main.go"

# Analisar CSV com reasoning
curl -X POST http://localhost:8080/chat \
  -F "prompt=Analise os dados e identifique tendências" \
  -F "model=claude-sonnet-4" \
  -F "reasoning_effort=high" \
  -F "file=@dados.csv"

# Multipart sem arquivo (somente texto — funciona igual ao JSON)
curl -X POST http://localhost:8080/chat \
  -F "prompt=Olá mundo" \
  -F "model=gpt-5-mini"
```

**Campos do multipart:**

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `prompt` | text | ✅ | Prompt/pergunta |
| `model` | text | ❌ | ID do modelo (default: `gpt-5-mini`) |
| `reasoning_effort` | text | ❌ | `low` / `medium` / `high` / `xhigh` |
| `file` | file | ❌ | Arquivo para análise (≤5MB) |

**Tipos de arquivo suportados (≤5MB):**

| Categoria | Extensões |
|-----------|-----------|
| Imagens | `.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`, `.bmp`, `.svg` |
| Texto | `.txt`, `.md`, `.json`, `.yaml`, `.yml`, `.csv`, `.xml`, `.html` |
| Código | `.go`, `.py`, `.js`, `.ts`, `.jsx`, `.tsx`, `.java`, `.c`, `.cpp`, `.h`, `.rs`, `.rb`, `.php`, `.sh`, `.sql`, `.css`, `.scss`, `.vue`, `.swift`, `.kt` |
| Documentos | `.pdf` |
| Dados | `.log`, `.env`, `.toml`, `.ini` |

> **Nota:** O arquivo temporário é automaticamente removido após o envio da resposta (via `defer cleanup()`).  
> Extensões não listadas retornam erro `400 validation_error`.

---

### WS /chat/stream (texto puro)

Streaming de tokens em tempo real via WebSocket. Cada fragmento é enviado imediatamente conforme gerado.

```javascript
const ws = new WebSocket("ws://localhost:8080/chat/stream");

ws.onopen = () => {
  ws.send(JSON.stringify({
    prompt: "Escreva um hello world em Go",
    model: "gpt-5-mini"
  }));
};

ws.onmessage = (event) => {
  const chunk = JSON.parse(event.data);
  switch (chunk.type) {
    case "chunk":     // fragmento de texto
      process.stdout.write(chunk.content);
      break;
    case "reasoning": // fragmento de raciocínio (thinking)
      process.stderr.write(chunk.content);
      break;
    case "done":      // geração concluída
      console.log("\n[Stream concluído]");
      ws.close();
      break;
    case "error":     // erro do SDK/modelo
      console.error("Erro:", chunk.error);
      ws.close();
      break;
  }
};
```

Via `wscat`:
```bash
wscat -c ws://localhost:8080/chat/stream
> {"prompt":"O que é uma goroutine?","model":"gpt-5-mini"}
< {"type":"chunk","content":"Uma goroutine"}
< {"type":"chunk","content":" é uma thread"}
< {"type":"chunk","content":" leve gerenciada pelo runtime do Go..."}
< {"type":"done"}
```

**Formato do request (JSON via WS):**

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `prompt` | string | ✅ | Prompt/pergunta |
| `model` | string | ❌ | ID do modelo (default: `gpt-5-mini`) |
| `reasoning_effort` | string | ❌ | `low` / `medium` / `high` / `xhigh` |
| `file` | object | ❌ | Arquivo anexo codificado em base64 |
| `file.data` | string | se `file` | Conteúdo em base64 |
| `file.mime_type` | string | se `file` | MIME type (ex: `image/png`) |
| `file.name` | string | se `file` | Nome com extensão (ex: `foto.png`) |

**Formato dos chunks (JSON via WS):**

| Campo | Tipo | Descrição |
|-------|------|-----------|
| `type` | string | `"chunk"` (texto), `"reasoning"` (thinking), `"done"` (fim), `"error"` (falha) |
| `content` | string | Fragmento de texto (quando `type` é `chunk` ou `reasoning`) |
| `error` | string | Mensagem de erro (quando `type` é `error`) |

---

### WS /chat/stream (com arquivo base64)

Envio de arquivo codificado em base64 junto com o prompt via WebSocket.  
O conteúdo é decodificado, salvo como arquivo temporário e passado ao SDK como `Attachment{Type: "file"}`.

```javascript
const fs = require("fs");
const imageBase64 = fs.readFileSync("foto.png").toString("base64");

const ws = new WebSocket("ws://localhost:8080/chat/stream");

ws.onopen = () => {
  ws.send(JSON.stringify({
    prompt: "Descreva esta imagem em detalhes",
    model: "gpt-4o",
    file: {
      data: imageBase64,
      mime_type: "image/png",
      name: "foto.png"
    }
  }));
};

ws.onmessage = (event) => {
  const chunk = JSON.parse(event.data);
  if (chunk.type === "chunk") process.stdout.write(chunk.content);
  if (chunk.type === "done") { console.log(); ws.close(); }
  if (chunk.type === "error") { console.error(chunk.error); ws.close(); }
};
```

> **Nota:** O limite de 5MB aplica-se ao conteúdo **decodificado** (não ao base64, que é ~33% maior).  
> O arquivo temporário é removido automaticamente após o streaming (via `defer cleanup()`).

---

## Detalhes Técnicos

### Streaming via Events (não polling)

O SDK **não** possui um método `Stream()`. O streaming é implementado via **event subscription**:

1. `session.On(callback)` — registra handler que recebe cada `SessionEvent`
2. `session.Send(ctx, opts)` — envia mensagem (assíncrono, retorna imediatamente)
3. Eventos `AssistantMessageDelta` chegam com `DeltaContent` (fragmentos de texto)
4. Eventos `AssistantReasoningDelta` chegam com `DeltaContent` (fragmentos de raciocínio)
5. Evento `SessionIdle` sinaliza que a geração terminou completamente

Os fragmentos são encaminhados via **channel Go** (buffer 128) para a goroutine do WebSocket, garantindo baixa latência e zero bloqueio.

### Upload de Arquivos

| Endpoint | Formato | Estratégia |
|----------|---------|------------|
| `POST /chat` | `multipart/form-data` | Arquivo salvo em `/tmp`, SDK recebe `Attachment{Type: "file", Path: tmpPath}` |
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
