# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/contentflow ./cmd/main.go

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=build /out/contentflow /app/contentflow
COPY internal/config/settings.yaml /app/internal/config/settings.yaml
COPY migrations /app/migrations

# mount point for Google Sheets service-account credentials (K8s Secret)
RUN mkdir /app/secrets

EXPOSE 8080
CMD ["/app/contentflow"]
