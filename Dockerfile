# --- Build stage ---
FROM golang:alpine AS builder

WORKDIR /src

# Copy all source (local replace directive requires full whatsmeow source)
COPY . .

RUN GOTOOLCHAIN=auto go build -trimpath -o /bin/walink ./cmd/walink

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
