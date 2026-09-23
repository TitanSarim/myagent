# LocalCode

Local AI coding CLI (Claude Code / Codex style) powered by Qwen models via Ollama.

**Milestone:** M1 — chat / ask / models / status with `fast` · `smart` · `code` · `auto` routing.

## Requirements

- [Ollama](https://ollama.com) running locally
- Models:
  - `qwen3.5:9b` → **fast**
  - `qwen3.8:27b` → **smart**
  - `qwen3.6:27b` → **code**
- Go 1.24+ (to build)

## Build

```bash
export PATH="$HOME/.local/go/bin:$PATH"   # if Go installed to ~/.local
cd "/home/sarim/Desktop/AI CLI"
go build -o bin/localcode ./cmd/localcode
```

## Quick start

```bash
./bin/localcode init
./bin/localcode models
./bin/localcode ask --mode fast "Explain what a mutex is in one paragraph"
./bin/localcode chat --mode auto
```

### Flags

| Flag | Meaning |
|------|---------|
| `--mode fast\|smart\|code\|auto` | Model profile |
| `--model <name>` | Override Ollama tag |
| `--context <n>` | Override `num_ctx` |
| `--provider-url` | Ollama base URL (default `http://127.0.0.1:11434`) |
| `-v` | Verbose |

### Chat slash commands

- `/mode fast|smart|code|auto`
- `/model <ollama-name>`
- `/status`
- `/clear`
- `/exit`

## Config

Written by `localcode init` to the OS config dir:

- Linux: `~/.config/localcode/config.yaml`
- macOS: `~/Library/Application Support/localcode/config.yaml`
- Windows: `%AppData%\localcode\config.yaml`

## Roadmap

See `LOCALCODE_FULL_PLAN.md` — next up: M2 repo read tools (git + ripgrep).
