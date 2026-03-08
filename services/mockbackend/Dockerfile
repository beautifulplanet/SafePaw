FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/mockbackend .

FROM alpine:3.21
RUN adduser -D -g '' safepaw
WORKDIR /app
COPY --from=builder /build/mockbackend /app/mockbackend
USER safepaw
EXPOSE 18789
HEALTHCHECK --interval=5s --timeout=2s --retries=3 \
    CMD wget -qO- http://localhost:18789/health || exit 1
ENTRYPOINT ["/app/mockbackend"]
