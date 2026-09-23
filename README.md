# myagent

**Open-source local AI coding agent** — Claude Code / Codex style, powered by Ollama + Qwen, with a bordered terminal UI.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go)](https://go.dev/)

## One-command install

### npm

```bash
npm install -g github:TitanSarim/myagent
myagent
```

(Once published to the npm registry: `npm install -g myagent`)

### curl (Linux / macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/TitanSarim/myagent/main/scripts/install.sh | bash
myagent
```

### Go

```bash
go install github.com/TitanSarim/myagent/cmd/localcode@latest
# optional: symlink/rename to myagent
```

## Requirements

- [Ollama](https://ollama.com) running locally
- Models (examples): `qwen3.5:9b`, `qwen3.8:27b`, `qwen3.6:27b`

```bash
ollama pull qwen3.5:9b
ollama pull qwen3.8:27b
ollama pull qwen3.6:27b
```

## Usage

```bash
myagent                 # open the terminal UI
myagent ask "…"         # one-shot question
myagent edit --dry-run --yes "…"
myagent --write         # chat with file edits enabled
```

### Terminal UI

- Bordered chat + live **CPU / RAM / GPU** bar
- Type `/` for commands (`/status`, `/tools`, `/mode`, …)
- `↑` `↓` — previous prompts (auto-copy)
- `Ctrl+↑` `Ctrl+↓` — browse chat messages (auto-copy)

## Features

| Area | Details |
|------|---------|
| Models | `fast` / `smart` / `code` / `auto` |
| Tools | read, search, git, `apply_patch`, `run_tests`, `run_command` |
| Safety | path sandbox, patch approval, command policy |
| Platforms | Linux · macOS · Windows |

## Build from source

```bash
git clone https://github.com/TitanSarim/myagent.git
cd myagent
go test ./...
./scripts/install.sh myagent
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Licensed under [MIT](LICENSE).

## Disclaimer

Runs models and tools on **your** machine. Review patches before applying. You are responsible for commands the agent runs.
