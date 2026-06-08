# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-w -s" -o nightdrive .

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app

COPY --from=builder /build/nightdrive /app/nightdrive
COPY --from=builder /build/internal/web/static /app/internal/web/static

# Create data directory
RUN mkdir -p /app/data

# Expose port
EXPOSE 4040

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:4040/api/health || exit 1

VOLUME ["/app/data", "/music"]

ENTRYPOINT ["/app/nightdrive"]
CMD ["-config", "/app/nightdrive.toml"]
