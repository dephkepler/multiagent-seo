# syntax=docker/dockerfile:1

ARG GO_VERSION=1.25
FROM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/createuser ./cmd/createuser

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=builder /out/server /app/server
COPY --from=builder /out/createuser /app/createuser
COPY --from=builder /src/migrations /app/migrations

EXPOSE 8080
ENTRYPOINT ["/app/server"]
