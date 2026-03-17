# ============================================================================
# Dockerfile — Multi-stage Build para API REST Copilot
#
# ESTRATÉGIA DE EMPACOTAMENTO DO COPILOT CLI:
# O SDK em Go (github.com/github/copilot-sdk/go) funciona acionando o
# executável da CLI do Copilot via JSON-RPC. O binário do CLI é um
# executável Node.js autocontido (~132MB) linkado contra glibc + libstdc++.
#
# Usamos o utilitário "bundler" do SDK para embutir o CLI no binário Go.
# Na inicialização, o SDK extrai a CLI para ~/.cache/copilot-sdk/.
#
# REQUISITOS DO AMBIENTE DE RUNTIME:
# O CLI precisa de glibc, libstdc++, libgcc e outras libs dinâmicas.
# Por esse motivo usamos debian:bookworm-slim como imagem final (~30MB)
# em vez de distroless/static (que não tem nenhuma lib dinâmica).
#
# ALTERNATIVA (volume mount, sem bundler):
# Monte o binário CLI como volume no Compose e defina COPILOT_CLI_PATH:
#   volumes:
#     - /usr/local/bin/copilot:/usr/local/bin/copilot:ro
#   environment:
#     - COPILOT_CLI_PATH=/usr/local/bin/copilot
# ============================================================================

# ── Fase 1: Build ───────────────────────────────────────────────────────────
# Usa imagem Debian (bookworm) porque o CLI embutido é linkado contra glibc.
# Alpine (musl) causa "no such file or directory" ao tentar exec o CLI.
FROM golang:bookworm AS builder

# Ferramentas mínimas de build.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copia go.mod e go.sum primeiro para cache de dependências.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copia todo o código-fonte.
COPY . .

# Usa o bundler do SDK para embutir o Copilot CLI no binário.
# O bundler baixa o CLI do npm (@github/copilot-linux-x64) e o embute
# via Go embed. O binário Go resultante extrai o CLI no primeiro uso.
RUN go get -tool github.com/github/copilot-sdk/go/cmd/bundler
RUN go tool bundler

# Compila o binário estático.
# Flags de otimização:
#   -s -w: remove tabela de símbolos e DWARF (reduz tamanho ~30%)
#   -trimpath: remove caminhos absolutos do binário
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /build/api-server .

# Pré-extrai o CLI embutido durante o build para que:
# 1) Verificamos que funciona em ambiente glibc
# 2) Copiamos o CLI já extraído para a imagem final (startup mais rápido)
RUN mkdir -p /tmp/cli-cache && HOME=/tmp/cli-cache \
    timeout 10 ./api-server 2>/dev/null || true && \
    ls -la /tmp/cli-cache/.cache/copilot-sdk/ 2>/dev/null || true

# ── Fase 2: Imagem Final ───────────────────────────────────────────────────
# debian:bookworm-slim (~30MB compressed) inclui:
#   - glibc, libstdc++, libgcc (necessários para o CLI do Copilot)
#   - Certificados SSL raiz
#   - Shell mínimo (útil para HEALTHCHECK e debugging)
FROM debian:bookworm-slim

# Metadados OCI.
LABEL maintainer="local-dev"
LABEL description="API REST + WebSocket para automação com GitHub Copilot SDK"

# Instala apenas certificados SSL (ca-certificates).
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Cria usuário não-root para segurança.
RUN groupadd -r appuser && useradd -r -g appuser -d /home/appuser -s /sbin/nologin appuser \
    && mkdir -p /home/appuser/.cache/copilot-sdk \
    && mkdir -p /tmp/copilot-uploads \
    && chown -R appuser:appuser /home/appuser \
    && chown appuser:appuser /tmp/copilot-uploads

# Copia o binário compilado (com CLI embutida).
COPY --from=builder /build/api-server /api-server

# Copia o CLI pré-extraído (se existir) para evitar extração no startup.
COPY --from=builder /tmp/cli-cache/.cache/copilot-sdk/ /home/appuser/.cache/copilot-sdk/
RUN chown -R appuser:appuser /home/appuser/.cache

# Define HOME para que o SDK encontre o CLI em ~/.cache/copilot-sdk/
ENV HOME=/home/appuser

# Porta padrão.
EXPOSE 8080

# Roda como usuário não-root.
USER appuser:appuser

# Health check nativo — verifica se a API responde.
HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=15s \
    CMD ["/bin/sh", "-c", "exec 3<>/dev/tcp/localhost/8080 && echo -e 'GET /health HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n' >&3 && cat <&3 | grep -q '\"ok\"'"]

ENTRYPOINT ["/api-server"]
