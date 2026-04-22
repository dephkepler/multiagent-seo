# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/contentflow ./cmd/main.go
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=build /out/contentflow /app/contentflow
COPY --from=build /out/migrate /app/migrate
COPY internal/config/settings.yaml /app/internal/config/settings.yaml
COPY migrations /app/migrations

EXPOSE 8080
CMD ["/app/contentflow"]
