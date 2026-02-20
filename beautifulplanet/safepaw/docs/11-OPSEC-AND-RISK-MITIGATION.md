# Safepaw: OPSEC & Risk Mitigation Strategy

## 1. The "Zero-Trust" Development Sandbox
To ensure your host machine (Windows) is never compromised and no local paths leak:
- **DevContainers:** All development will happen inside an isolated Docker container. The code will not run directly on your Windows machine.
- **No Host Volume Mounts for Secrets:** Secrets will be injected via a `.env` file that is strictly `.gitignore`d.
- **Network Isolation:** The development container will only have access to explicitly defined ports.

## 2. Code & Repository OPSEC
- **Pre-commit Secret Scanning:** We will implement `trufflehog` or `git-secrets` to scan every commit before it leaves your machine. If it detects an API key, email, or absolute Windows path (e.g., `C:\Users\...`), the commit will be blocked.
- **Aggressive Gitignore:** We have already established a strict `.gitignore`.
- **No Real Data:** We will NEVER use your personal Discord, WhatsApp, or Telegram accounts for testing. We will create "burner" accounts specifically for Safepaw development.

## 3. Architectural Risk Mitigation (The Pivot)
The original "Ivy League" architecture proposed 4 languages (Rust, Go, Python, Node.js). **This is a massive execution risk.**
To eliminate unknowns and guarantee delivery, we must simplify:
- **Consolidated Stack:** We will use **Go** for the core infrastructure (Gateway + Router) for safety and concurrency, and **TypeScript/Node.js** for the Agent/Plugins (since we are extracting from OpenClaw). 
- **Why?** Fewer languages = fewer unknowns, easier debugging, and zero cross-language serialization nightmares.

## 4. LLM & Agent Sandboxing (Production OPSEC)
When the AI agent executes code or tools:
- **No Host Access:** The agent will run in a locked-down Docker container with `read-only` file systems.
- **Egress Filtering:** The agent container will ONLY be allowed to make outbound network requests to approved LLM APIs (OpenAI, Anthropic). It cannot browse the open web unless explicitly using a sandboxed browser tool.
- **Ephemeral State:** Every agent session starts fresh. No cross-contamination of memory.

## 5. Operational Rules for this Hackathon
1. **Never paste real API keys into our chat.** Use placeholders like `sk-ant-xxxxxx`.
2. **Never share real user data.**
3. **If a task feels too complex, we stop and break it down further.**
