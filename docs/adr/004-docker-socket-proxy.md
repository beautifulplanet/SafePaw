# ADR-004: Docker Socket Proxy Instead of Direct Mount

**Status:** Accepted  
**Date:** 2026-03-01  
**Deciders:** Project leads  

## Context

The wizard dashboard needs to list running containers and restart services.
This requires access to the Docker Engine API. The standard approach is to
mount the Docker socket directly into the container:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock
```

**Problem:** The Docker socket is equivalent to root access on the host.
Any process that can talk to the Docker socket can:

- Start new privileged containers
- Mount the host filesystem
- Execute commands inside any container
- Pull arbitrary images from any registry
- Read secrets from any running container

If the wizard is ever compromised (e.g., via an XSS, dependency vulnerability,
or SSRF), the attacker gains full control of the host machine. This violates
the principle of least privilege — the wizard only needs to list containers and
restart them, not the full Docker API.

## Decision

Use [tecnativa/docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy)
as an intermediary. The proxy whitelists specific Docker API calls and blocks
everything else.

Configuration in `docker-compose.yml`:

```yaml
docker-socket-proxy:
  image: tecnativa/docker-socket-proxy:0.3
  environment:
    CONTAINERS: 1   # Allow list + inspect + restart
    POST: 1         # Allow POST requests (restart)
    # All other categories default to 0 (blocked):
    # IMAGES: 0, NETWORKS: 0, VOLUMES: 0, EXEC: 0,
    # BUILD: 0, SERVICES: 0, TASKS: 0, CONFIGS: 0, SECRETS: 0
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock:ro
  networks:
    - safepaw-internal
```

The wizard connects to the proxy instead of the socket:

```yaml
wizard:
  environment:
    DOCKER_HOST: tcp://docker-socket-proxy:2375
```

**Allowed operations (4 total):**
1. `GET /_ping` — health check
2. `GET /containers/json` — list containers
3. `GET /containers/{id}/json` — inspect a container
4. `POST /containers/{id}/restart` — restart a container

**Blocked operations (everything else):**
- `POST /containers/create` — cannot create new containers
- `POST /exec/{id}/start` — cannot exec into containers
- `POST /build` — cannot build images
- `GET /secrets` — cannot read Docker secrets
- `DELETE /containers/{id}` — cannot remove containers

## Consequences

**Good:**
- **Blast radius reduction** — A compromised wizard can only list and restart
  containers. It cannot escape to the host, read secrets, or start new containers.
- **Defense in depth** — Even if both the wizard AND the proxy are compromised,
  the socket is mounted read-only (`:ro`).
- **Audit trail** — The proxy logs all API calls, providing visibility into
  what the wizard is actually doing.
- **Network isolation** — The proxy runs on an internal Docker network. No
  external access.
- **Drop-in** — No code changes needed. The Docker client library respects
  `DOCKER_HOST` transparently.

**Bad:**
- **Extra container** — One more service to manage (though it's tiny: ~5 MB
  image, <10 MB RAM).
- **Latency** — One extra network hop per Docker API call. Negligible for
  dashboard use (list + restart are rare operations).
- **Version coupling** — Must track docker-socket-proxy releases. Pinned to
  0.3 to avoid surprises.

**Neutral:**
- This pattern is recommended by Docker's official security documentation and
  used by production projects like Traefik, Portainer (optional), and Lazydocker.

## References

- `docker-compose.yml` — Proxy service definition (lines 85–114)
- `docker-compose.demo.yml` — Same pattern for demo stack
- [Docker socket security](https://docs.docker.com/engine/security/#docker-daemon-attack-surface) — Official docs
- [Tecnativa docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) — Upstream project
