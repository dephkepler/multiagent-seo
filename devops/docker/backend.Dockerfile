# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25
FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
# Cache mounts persist across builds (not just across layers in one build) —
# without them every deploy recompiled the whole module graph from scratch,
# ~13+ minutes on the VPS's modest CPU. GOCACHE/module cache now survive
# between `dc up --build`, so only changed packages actually recompile.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/createuser ./cmd/createuser

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=builder /out/server /app/server
COPY --from=builder /out/createuser /app/createuser
COPY --from=builder /src/migrations /app/migrations

EXPOSE 8080
ENTRYPOINT ["/app/server"]
