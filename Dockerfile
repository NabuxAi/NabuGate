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

# No config is baked in: that would publish the example API key as a live,
# full-access credential. Operators must mount their own config at /app/config.yaml
# (see docker-compose.yml). The gateway fails to start if it is missing.
ENV NABU_CONFIG=/app/config.yaml
EXPOSE 8080

ENTRYPOINT ["/app/nabugate"]
