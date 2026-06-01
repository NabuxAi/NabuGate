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
# A starter config; mount/override with your own in production.
COPY config.example.yaml /app/config.yaml

ENV NABU_CONFIG=/app/config.yaml
EXPOSE 8080

ENTRYPOINT ["/app/nabugate"]
