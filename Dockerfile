# Build stage
FROM golang:alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o sonata ./cmd/sonata
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.27.2

# Runner stage
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/sonata .
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY --from=builder /app/config ./config
COPY --from=builder /app/protected ./protected
COPY --from=builder /app/internal/database/migrations ./internal/database/migrations

RUN mkdir -p /etc/secrets && echo "Authorization: Bearer mock" > /etc/secrets/grafana-otlp-headers

EXPOSE 8080
ENTRYPOINT ["./sonata"]
CMD ["api", "--config-root", "./config", "--profile", "production"]
