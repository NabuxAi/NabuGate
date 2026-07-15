# syntax=docker/dockerfile:1

############################
# Stage 1: Build
############################
FROM golang:1.24-bookworm AS build

WORKDIR /src

# Cache module downloads
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/nabugate ./cmd/gateway

############################
# Stage 2: Runtime
############################
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/nabugate /app/nabugate

# A secret-free default config is baked in so the gateway boots out of the box:
# it ships no keys — the admin key comes from ${NABU_API_KEY} and provider
# secrets from their own env vars. Override it by mounting your own file at
# /app/config.yaml or by setting NABU_CONFIG_YAML (either wins over this default).
COPY config.default.yaml /app/config.yaml
# The cinematic-scrollytelling sub-agents; the default config loads them via
# agents_dir: "/app/agents".
COPY agents /app/agents
ENV NABU_CONFIG=/app/config.yaml
EXPOSE 8080

ENTRYPOINT ["/app/nabugate"]
