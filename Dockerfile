FROM golang:1.23-alpine AS builder

WORKDIR /src

# leverage layer cache for deps
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# CGO disabled for static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /out/api /app/api
COPY --chown=app:app configs ./configs
COPY --chown=app:app migrations ./migrations

USER app

EXPOSE 8080
ENTRYPOINT ["/app/api"]
