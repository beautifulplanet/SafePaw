# Mock Backend

Minimal HTTP server for **testing the gateway without OpenClaw**. Use with integration tests or scripts that point the gateway at this server (e.g. `PROXY_TARGET=http://localhost:18789`).

## Build and run

```bash
cd services/mockbackend
go build -o mockbackend .
./mockbackend                    # listen :18789
# or
PORT=9999 ./mockbackend          # custom port
```

## Endpoints

| Path | Method | Description |
|------|--------|-------------|
| `/health` | GET | 200 OK — gateway health check |
| `/echo` | GET, POST, PUT | Echo query, headers, body as JSON |
| `/status/{code}` | GET | Return HTTP status (e.g. `/status/500`) |
| `/payload/injection` | GET | Body that triggers gateway prompt-injection scanner |
| `/payload/xss` | GET | Body that triggers gateway output scanner (XSS) |
| `/delay?ms=N` | GET | Respond after N ms (timeout testing) |

## Use with gateway

```bash
# Terminal 1
./mockbackend

# Terminal 2
cd services/gateway
PROXY_TARGET=http://localhost:18789 AUTH_ENABLED=false ./gateway

# Then: curl http://localhost:8080/health ; curl http://localhost:8080/echo
```

See `scripts/integration-gateway-mock.sh` for an automated run.
