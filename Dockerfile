# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY static/ static/
RUN go build -o nightdrive .

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ffmpeg ca-certificates sqlite-libs

WORKDIR /app

COPY --from=builder /app/nightdrive /app/nightdrive
COPY --from=builder /app/static /app/static

ENV NIGHTDRIVE_MUSIC=/music
ENV PORT=8080

VOLUME ["/music", "/data"]

EXPOSE 8080

CMD ["/app/nightdrive"]
