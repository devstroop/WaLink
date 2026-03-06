# --- Build stage ---
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Copy dependency files first for better layer caching
COPY go.mod go.sum ./
COPY whatsmeow/ ./whatsmeow/

RUN go mod download

# Copy source code
COPY . .

RUN go build -trimpath -o /bin/walink ./cmd/walink

# --- Runtime stage ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S walink && adduser -S walink -G walink

COPY --from=builder /bin/walink /usr/local/bin/walink

# Default data directory
RUN mkdir -p /data/db /data/accounts && chown -R walink:walink /data

USER walink
WORKDIR /home/walink

EXPOSE 3000

ENTRYPOINT ["walink"]
