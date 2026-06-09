# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/createuser ./cmd/createuser

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=build /out/server /app/server
COPY --from=build /out/createuser /app/createuser

EXPOSE 8080
CMD ["/app/server"]
