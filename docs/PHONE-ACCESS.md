# Phone / mobile access to OpenClaw via SafePaw

You can let people use their **phone** (or any device) to talk to OpenClaw through SafePaw. They connect to the **gateway** with a token; the gateway proxies to OpenClaw. The wizard stays on your machine (admin only).

---

## How it works

| Component | Role for phone users |
|-----------|----------------------|
| **Wizard** (:3000) | Admin only. You use it to create tokens and manage the stack. Kept on localhost. |
| **Gateway** (:8080) | Entry point for users. Phone → gateway (with token) → OpenClaw. You expose this to the network the phone can reach. |

Out of the box, both wizard and gateway are bound to **127.0.0.1** (localhost only). To allow phones, you expose the gateway and configure auth and CORS.

---

## Steps to enable phone access

### 1. Make the gateway reachable from the phone

**Same WiFi (e.g. home/office):**

- In `docker-compose.yml`, change the gateway port mapping from:
  ```yaml
  - "127.0.0.1:8080:8080"
  ```
  to:
  ```yaml
  - "8080:8080"
  ```
  so the gateway listens on all interfaces. Restart the stack.
- On the phone, use `http://<your-pc-ip>:8080` (e.g. `http://192.168.1.10:8080`). Find your PC’s IP with `ipconfig` (Windows) or `ifconfig` / `ip a` (Linux/macOS).

**Internet (phone not on same LAN):**

- Put the gateway behind a reverse proxy (Caddy, nginx, etc.) or a tunnel (Tailscale, ngrok) that has a public hostname and TLS. Point the phone at that URL. Keep the wizard on localhost.

### 2. Turn on gateway auth and create tokens

- In `.env` (or the env passed to the gateway):
  - `AUTH_ENABLED=true`
  - `AUTH_SECRET=<secret>` (e.g. `openssl rand -base64 48`)
- Restart the stack.
- Create a token for each user (or app):
  - **From the wizard:** sign in at http://localhost:3000 → Settings or the token UI (if available), generate a token and copy it.
  - **From the host:**  
    `cd services/gateway && AUTH_SECRET=<same-secret> go run ./tools/tokengen -sub phone-user -scope proxy`
- Give the user the token (securely). The phone uses it in requests (see below).

### 3. CORS (only if the phone uses a **browser**)

- If the phone opens a **web page** that calls the gateway (e.g. a PWA or a web chat UI), the browser enforces CORS. Set the gateway’s allowed origins so that page’s origin is allowed.
- In `.env` for the gateway:
  - `ALLOWED_ORIGINS=https://your-app.example.com,http://192.168.1.10:3000`
  (use the exact origin of the page the phone loads).
- If the phone uses a **native app** or direct HTTP/WebSocket (no browser), CORS does not apply; the app just needs the gateway URL and token.

### 4. Use TLS when the phone is not on a trusted LAN

- On the same WiFi, HTTP is often acceptable for testing.
- For internet or untrusted networks, use HTTPS so the token is not sent in the clear. Enable TLS on the gateway (`TLS_ENABLED=true` and certs) or, more commonly, put the gateway behind a reverse proxy or tunnel that terminates TLS.

---

## How the phone sends the token

- **HTTP:**  
  `Authorization: Bearer <token>`  
  Example: `curl -H "Authorization: Bearer <token>" http://<gateway>/echo`
- **WebSocket (e.g. chat):**  
  Many setups pass the token in the URL: `ws://<gateway>/ws?token=<token>`  
  (See gateway/tokengen docs for the exact query param name if it differs.)

The phone app or web page must send the token on every request (or on WebSocket connect) so the gateway accepts the connection and proxies to OpenClaw.

---

## Minimal “same WiFi” checklist

1. Change gateway port mapping to `8080:8080` in `docker-compose.yml`; restart.
2. Set `AUTH_ENABLED=true` and `AUTH_SECRET` in `.env`; restart.
3. Generate a token (wizard or tokengen); give it to the user.
4. On the phone, use base URL `http://<your-pc-ip>:8080` and send the token (e.g. `Authorization: Bearer <token>` or `?token=<token>` for WebSocket).
5. If the client is a browser app, set `ALLOWED_ORIGINS` to that app’s origin.

---

## Security notes

- **Wizard:** Keep it on localhost. Do not expose the wizard to the internet; it’s for admin only.
- **Tokens:** Treat them like passwords. Use HTTPS (or a trusted LAN) when sending them. Rotate if compromised (see RUNBOOK).
- **Rate limits:** The gateway applies per-IP rate limits and brute-force protection; phones behind NAT may share an IP.

For hardening (TLS, revocation, runbooks), see [SECURITY.md](../SECURITY.md) and [RUNBOOK.md](../RUNBOOK.md).
