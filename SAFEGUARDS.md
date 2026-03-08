# SafePaw / InstallerClaw — Safeguards

Lessons from a session that froze a Windows machine. Follow these before running the stack.

---

## Before running `docker compose up -d`

### 1. Set Docker Desktop resource limits

Open Docker Desktop > Settings > Resources:

| Setting | Recommended |
|---------|-------------|
| CPUs    | 2 (not "max") |
| Memory  | 4 GB max |
| Swap    | 1 GB |
| Disk    | 40 GB max |

This prevents Docker from eating all your RAM and CPU.

### 2. Turn off your VPN first (or configure it)

Docker creates virtual network adapters that conflict with most VPNs on Windows. Either:
- Disconnect VPN before starting the stack, reconnect after
- Or add Docker's subnet (172.16.0.0/12) to your VPN's split-tunnel / bypass list

### 3. Check disk space

You need at least 5 GB free. Check: `Get-PSDrive C` in PowerShell.

### 4. Check ports are free

Ports 3000 and 8080 must not be in use:
```powershell
netstat -ano | findstr ":3000 :8080"
```
If something is using them, stop that app first.

---

## While running

### 5. Don't rebuild images repeatedly

Each OpenClaw build downloads ~650 packages. Build once; if it fails, fix the Dockerfile first, then build once more. Never use `--no-cache` unless absolutely necessary.

### 6. Stop crash-looping containers immediately

If a container keeps restarting:
```powershell
docker compose stop openclaw
docker logs safepaw-openclaw 2>&1 | Select-Object -Last 20
```
Fix the issue BEFORE restarting. Don't let it loop.

### 7. Monitor resources

Keep Task Manager open (Ctrl+Shift+Esc > Performance tab) while the stack is starting. If `vmmem` (Docker's WSL2 process) exceeds 4 GB RAM or CPU is pinned at 100% for more than 2 minutes, run:
```powershell
docker compose down
```

---

## Emergency stop

If your machine freezes or becomes unresponsive:

1. Ctrl+Alt+Del > Task Manager
2. Find `Docker Desktop` and End Task
3. Find `vmmem` and End Task (this kills WSL2/Docker)
4. Everything stops immediately. Your machine will recover.

---

## Full cleanup

Remove everything Docker-related (images, volumes, cache):
```powershell
docker compose down
docker system prune -a -f --volumes
```

---

## What happened and root causes

| Symptom | Cause |
|---------|-------|
| Computer froze | Docker used all available RAM/CPU (no resource limits set) |
| Lost internet | Docker virtual network adapter conflicted with VPN routing |
| Strange processes | Docker internals: vmmem (WSL2), com.docker.backend, buildkit |
| Disk space warnings | 4+ image rebuilds filled build cache (~4.6 GB) |

None of these are security breaches. All are local Docker resource conflicts.
