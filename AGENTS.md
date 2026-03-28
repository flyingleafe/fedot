# AGENTS.md

This file provides guidance to coding agents when working with code in this repository.

## Workflow

**Before making any code change** (bug fix, feature, refactor): invoke the `improve-picoclaw` skill. It ensures the local branch is synced with upstream, checks for existing PRs addressing the same issue, and guides PR creation. Skip only for documentation-only edits.

## Commands

```bash
make build          # build for current platform → build/picoclaw-linux-amd64
make install        # build + install to ~/.local/bin/picoclaw
make test           # run all Go tests (excludes web/)
make lint           # run golangci-lint
make fmt            # format with golangci-lint fmt
make fix            # auto-fix lint issues
make check          # deps + fmt + vet + test

# Single test
go test -v -tags goolm,stdjson ./pkg/config -run TestConfigLoad

# Minimal connection test after provider changes
picoclaw agent -m "hi" --model <model-name>
```

CGO is disabled (`CGO_ENABLED=0`). Build tags default to `goolm,stdjson`.

## Architecture

PicoClaw is an ultra-lightweight AI agent server (<10MB RAM) written in Go. Requests flow: **channel → bus → agent loop → provider**.

### Request Flow

1. A **channel** (`pkg/channels/`) receives a message and publishes it to the in-memory bus (`pkg/bus/`)
2. The **agent loop** (`pkg/agent/loop.go`) dequeues by session key, loads conversation history from the **session store** (`pkg/session/`), and builds LLM context
3. The loop calls the **LLM provider** (`pkg/providers/`), handles tool calls by executing them and looping, then publishes the response back to the bus
4. The **channel manager** delivers the response to the user, with streaming if supported

The **gateway** (`pkg/gateway/`) owns startup, config loading, channel/agent wiring, and graceful shutdown.

### Provider System

`CreateProviderFromConfig()` in `pkg/providers/factory_provider.go` is the factory entry point. It extracts the protocol prefix from `model_list[].model` (e.g. `"openrouter/xiaomi/mimo-v2-pro"` → protocol `openrouter`, modelID `xiaomi/mimo-v2-pro`) and dispatches to the correct provider constructor.

- **OpenAI-compatible HTTP**: `openai/`, `openrouter/`, `groq/`, `gemini/`, `ollama/`, `deepseek/`, `qwen/`, and ~20 more — all go through `NewHTTPProvider*`
- **Auth-store providers**: `openai` and `anthropic` with `auth_method: oauth|token` read credentials from `~/.picoclaw/auth.json` via `pkg/auth/`; `openrouter` with `auth_method: token` does the same via `createOpenRouterAuthProvider()`
- **Native SDK**: `anthropic/` with no auth_method uses HTTP with API key; `bedrock/` uses AWS SDK
- **Fallback chain**: `model_list[].fallbacks` lists model names to try in order when the primary fails

Adding a new provider: add its protocol to the switch in `CreateProviderFromConfig`, add a default API base in `getDefaultAPIBase`, and (if it needs auth-store login) add a case in `cmd/picoclaw/internal/auth/helpers.go`.

### Config & Credentials

- **Config**: `~/.picoclaw/config.json` — loaded and migrated on startup by `pkg/config/`
- **Secrets**: `~/.picoclaw/.security.yml` — API keys kept separate from config; merged at load time via `SecureString` types that support `enc://` (AES-GCM) and `file://` references
- **Auth store**: `~/.picoclaw/auth.json` — OAuth/token credentials written by `picoclaw auth login`; read at request time by provider factory

`ModelConfig.APIKeys` (`SecureStrings`) holds static keys from config/security files. When `AuthMethod` is `"token"` or `"oauth"`, the factory ignores `APIKeys` and reads from the auth store instead.

Multiple keys in `api_keys` are automatically expanded into virtual fallback entries (`pkg/config/config.go`, `expandMultiKeyModels`).

### Login Flow

`picoclaw auth login --provider <name>` saves a credential to auth.json and stamps `auth_method` on all matching models in config. Supported providers: `openai`, `anthropic`, `google-antigravity`, `openrouter`. Implementation in `cmd/picoclaw/internal/auth/helpers.go`.

### Session & Memory

Sessions are keyed `channel:chatID`. History is stored as JSONL per session in `~/.picoclaw/sessions/`. When context exceeds the configured limit, the loop summarizes older turns. Long-term memory lives separately in `pkg/memory/`.

### Tools & Skills

Tools are registered in `pkg/tools/` and available to the agent loop. Built-ins include shell execution, file read/write, web search, cron scheduling, and MCP protocol integration (`pkg/mcp/`). Skills are Markdown files loaded from configured directories and injected as tool context.

## Self-Improvement Mode

When invoked via the `/selfimprove` Telegram command you are operating in **self-improvement mode**. The picoclaw process launched you via `claude -p <prompt> --permission-mode auto`. Your stdout/stderr are captured and forwarded to the requesting user.

Follow this workflow exactly:

1. Understand the request and make the necessary code changes.
2. Run `make build` to verify the code compiles. Fix any errors and rebuild until it passes.
3. Run `make install` to install the updated binary to `~/.local/bin/picoclaw`.
4. Run `systemctl --user restart picoclaw` to restart the service with the new binary.
5. Run `systemctl --user is-active picoclaw` to confirm the service came back up.
6. Print a concise summary of what you changed and the outcome.

**Important rules:**
- **Always commit your changes** before running `make install`. The worktree may contain other uncommitted work; if self-improvement fails, the caller reverts to the pre-attempt HEAD, which would destroy any uncommitted changes.
- Do NOT restart the service unless both `make build` and `make install` succeed.
- Print progress clearly — the user sees your stdout in real time.
- If any step fails, print the error and exit with a non-zero code so the caller can revert and retry.
- Keep changes minimal and focused on the stated request.
- Do NOT modify CLAUDE.md, config files, or credentials during self-improvement.
