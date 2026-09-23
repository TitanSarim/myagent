# LocalCode — Full Product Plan & Tech Stack

**Product:** Local AI coding CLI (Claude Code / Codex / Cursor-agent style)  
**Models:** Qwen3.5 9B · Qwen3.8 27B · Qwen3.6 27B Coding (via Ollama)  
**Platforms:** Linux · Windows · macOS (and limited iOS options — see §3)  
**Document version:** September 2026  
**Hardware baseline (dev workstation):** Linux, 32 GB RAM, RTX A2000 8 GB

---

## 1. Product vision

Build a **local-first coding agent CLI** that:

- Runs entirely on-device (no cloud required for inference)
- Understands a Git repository (search, symbols, diffs, history)
- Edits via **validated unified diffs** with approval gates
- Runs tests, inspects failures, and iterates in a bounded agent loop
- Routes each request to the right Qwen model (`fast` / `smart` / `code` / `auto`)
- Ships as a **single native binary** on Linux, Windows, and macOS

**Feel target:** like Claude Code / Codex CLI / Cursor agent — not a thin chat wrapper around Ollama.

```text
$ cd ~/projects/my-service
$ localcode
LocalCode  ·  mode:auto → code  ·  qwen3.6:27b-coding  ·  my-service

> add pagination to the orders endpoint and update tests

Plan
1. inspect routing + handler
2. find pagination patterns
3. patch impl + tests
4. run targeted tests
5. show final diff
```

---

## 2. Model strategy (fixed)

| Mode | Ollama tag | ~Disk | Role |
|------|------------|-------|------|
| `fast` | `qwen3.5:9b` | ~6.6 GB | Explanations, small edits, quick Q&A |
| `smart` | `qwen3.8:27b` | ~18 GB | Architecture, hard debugging, planning |
| `code` | `qwen3.6:27b-coding` | ~18 GB | Multi-file impl, refactors, agent loops |
| `auto` | router picks one | — | One model per request |

**Hard rule:** one loaded model at a time (`OLLAMA_MAX_LOADED_MODELS=1`). Do not bounce between 27B models mid-request.

**Context defaults (8 GB VRAM):**

| Model | Start | Stretch |
|-------|-------|---------|
| 9B | 16K | higher if RAM healthy |
| 27B ×2 | 8K | 16K only if stable |

Retrieval beats giant context: search → rank → send only high-signal snippets.

---

## 3. Cross-platform support (Windows · Linux · macOS · iOS)

### 3.1 What “CLI on every OS” means

| Platform | Support level | Notes |
|----------|---------------|-------|
| **Linux** | Primary / first-class | Best path for Ollama + NVIDIA; your build machine |
| **Windows** | First-class | Native binary + WinGet; Ollama Windows app; path/`\` handling |
| **macOS** | First-class | Apple Silicon / Intel; Metal via Ollama; Homebrew formula |
| **iOS** | Not a native CLI target | iOS has no general shell CLI store product. Offer **alternatives** below |

Clarification: if you meant **iOS = iPhone/iPad**, a full Claude-like coding agent CLI is not shippable as a normal App Store binary with local 27B models. If you meant **macOS**, that is fully in scope as first-class.

### 3.2 Recommended platform matrix

```text
                    ┌─────────────────────────────────────┐
                    │         localcode (Go binary)       │
                    │  linux/amd64  windows/amd64         │
                    │  darwin/amd64  darwin/arm64         │
                    └─────────────────┬───────────────────┘
                                      │ HTTP
                    ┌─────────────────▼───────────────────┐
                    │     Ollama (or OpenAI-compat API)   │
                    │  Linux systemd · Windows service    │
                    │  macOS launchd · remote URL OK      │
                    └─────────────────┬───────────────────┘
                                      │
              ┌───────────────────────┼───────────────────────┐
              ▼                       ▼                       ▼
        qwen3.5:9b              qwen3.8:27b           qwen3.6:27b-coding
```

### 3.3 OS-specific runtime requirements

| Concern | Linux | Windows | macOS |
|---------|-------|---------|-------|
| Shell / process | `/bin/sh` + argv (no shell interp by default) | `cmd` / PowerShell only when needed; prefer argv | `/bin/zsh` or `/bin/sh` |
| Paths | POSIX | Use `filepath` always; Git Bash optional | POSIX |
| Line endings | LF | Normalize CRLF in patches | LF |
| Secrets dir | `~/.config/localcode` | `%AppData%\localcode` | `~/Library/Application Support/localcode` |
| Model store | `/var/lib/ollama-models` (custom) | Ollama default under user profile | Ollama default |
| GPU | CUDA (NVIDIA) | CUDA / DirectML via Ollama | Metal |
| Packaging | `.tar.gz` + apt/rpm later | `.zip` + WinGet | `.tar.gz` + Homebrew |

### 3.4 iOS options (if you still want mobile)

Pick **one** secondary track later — do not block the desktop CLI:

1. **Remote agent mode (recommended):** iPhone SSH / Termius / Blink into a Linux box running `localcode` + Ollama.
2. **Companion app (Phase 2+):** iOS SwiftUI app that talks to a **home server** API wrapping the same agent (no on-device 27B).
3. **a-Shell / iSH:** experimental only; no serious local 27B; not a product path.

**Decision:** Ship **Linux + Windows + macOS** as v1. Treat iOS as remote client / companion later.

---

## 4. Tech stack (recommended)

### 4.1 Core (locked)

| Layer | Choice | Why |
|-------|--------|-----|
| Language | **Go 1.23+** | One static binary, fast start, great FS/process/HTTP, easy cross-compile |
| CLI framework | **Cobra** + **Viper** | Subcommands + config (Claude/Codex-like UX) |
| TUI (optional M10) | **Bubble Tea** + **Lip Gloss** | Interactive sessions, spinners, diff panes |
| LLM runtime | **Ollama** | Local models, simple HTTP API, Windows/macOS/Linux |
| Search | **ripgrep** (external binary) or `go-grep` fallback | Speed + familiar UX |
| VCS | **git** CLI + `go-git` for parsing where useful | Status/diff/branch |
| Patching | Custom **unified-diff** validate/apply | Safer than whole-file rewrite |
| Config | YAML (`config.yaml`) + env overrides | Portable |
| Logging | `slog` + JSONL session logs | Debuggable agent loops |
| Testing | `testing` + `testify` + temp Git fixtures | Deterministic tool loops with mocked LLM |

### 4.2 Provider abstraction (mandatory)

```go
type Provider interface {
    Chat(ctx context.Context, req ChatRequest) (<-chan Event, error)
    ListModels(ctx context.Context) ([]ModelInfo, error)
}
```

Implementations:

1. `provider/ollama` — default  
2. `provider/openai_compat` — future (vLLM, llama.cpp server, cloud)  
3. `provider/mock` — tests  

### 4.3 Optional later (not v1)

| Piece | When |
|-------|------|
| Tree-sitter (via `go-tree-sitter`) | After lexical search is proven |
| Embeddings (sqlite-vec / chroma local) | Only if retrieval quality plateaus |
| SQLite session index | When JSONL sessions get large |
| Fine-tuning | Only after tool/prompt gaps are measured |

### 4.4 Cross-compile & CI

```bash
# Example release matrix
GOOS=linux   GOARCH=amd64  go build -o dist/localcode-linux-amd64
GOOS=linux   GOARCH=arm64  go build -o dist/localcode-linux-arm64
GOOS=windows GOARCH=amd64  go build -o dist/localcode-windows-amd64.exe
GOOS=darwin  GOARCH=amd64  go build -o dist/localcode-darwin-amd64
GOOS=darwin  GOARCH=arm64  go build -o dist/localcode-darwin-arm64
```

CI: **GitHub Actions** matrix build + unit tests on all three OS runners.  
Release: **GoReleaser** → GitHub Releases + checksums + Homebrew tap + WinGet PR.

### 4.5 Dependencies the user must install

| Dependency | Required? | Notes |
|------------|-----------|-------|
| Ollama | Yes (local mode) | Or point `base_url` at a remote server |
| Git | Yes | Repo discovery |
| ripgrep | Strongly recommended | CLI can vendor or download helper |
| Language toolchains | Optional | Only for `run_tests` (go/npm/pytest/…) |

---

## 5. Architecture

```text
localcode CLI
│
├── Session / UX  (REPL, streaming, approvals, progress)
│
└── Agent Controller
      ├── Context Engine   → git ls-files, rg, symbols, diff, history summary
      ├── Model Router     → fast | smart | code | auto (one pick / request)
      └── Tool Engine      → read / search / patch / test / shell / git
                │
                ▼
         LLM Provider (Ollama)
                │
     ┌──────────┼──────────┐
     ▼          ▼          ▼
  Qwen3.5 9B  Qwen3.8 27B  Qwen3.6 27B Coding
```

### 5.1 Suggested repo layout

```text
localcode/
├── cmd/localcode/
│   └── main.go
├── internal/
│   ├── agent/          # loop, planner, step budget
│   ├── provider/       # ollama + mock + openai_compat
│   ├── router/         # fast/smart/code/auto
│   ├── contextx/       # ranking, token budget, summarizer
│   ├── tools/          # typed tools + registry
│   ├── patch/          # parse / validate / apply unified diffs
│   ├── security/       # path sandbox, command policy, secrets deny
│   ├── session/        # conversation + actions + patches
│   ├── repo/           # git root discovery, ignore rules
│   ├── config/         # load/merge YAML + XDG/AppData paths
│   └── ui/             # streaming printer; later Bubble Tea
├── prompts/
│   ├── system.md
│   ├── planner.md
│   └── reviewer.md
├── testdata/fixtures/  # tiny repos for agent benchmarks
├── .goreleaser.yaml
├── go.mod
└── README.md
```

### 5.2 Config paths (cross-platform)

Use Go’s `os.UserConfigDir` / `os.UserCacheDir` / `os.UserHomeDir`:

| Data | Linux | Windows | macOS |
|------|-------|---------|-------|
| Config | `~/.config/localcode/config.yaml` | `%AppData%\localcode\config.yaml` | `~/Library/Application Support/localcode/config.yaml` |
| Sessions | `~/.local/share/localcode/` | `%LocalAppData%\localcode\` | `~/Library/Application Support/localcode/` |
| Repo state | `<repo>/.localcode/` | same | same |

Example `config.yaml`:

```yaml
provider:
  type: ollama
  base_url: http://127.0.0.1:11434

models:
  fast:
    name: qwen3.5:9b
    context: 16384
    keep_alive: 2m
  smart:
    name: qwen3.8:27b
    context: 8192
    keep_alive: 2m
  code:
    name: qwen3.6:27b-coding
    context: 8192
    keep_alive: 2m

routing:
  default: auto
  one_model_per_request: true

safety:
  require_patch_approval: true
  require_dangerous_command_approval: true
  restrict_to_repo: true
```

---

## 6. CLI command surface

| Command | Purpose |
|---------|---------|
| `localcode` / `localcode chat` | Interactive repo-aware REPL |
| `localcode ask "..."` | One-shot Q&A |
| `localcode explain <path>` | Explain file/dir/symbol |
| `localcode plan "..."` | Plan only (no writes) |
| `localcode edit "..."` | Propose patch (approval) |
| `localcode fix "..."` | Diagnose → patch → test loop |
| `localcode test` | Run relevant tests + summarize |
| `localcode review` | Review current git diff |
| `localcode models` | Show mappings + Ollama health |
| `localcode status` | Repo, mode, GPU/runtime hints |
| `localcode index` | Refresh optional symbol index |
| `localcode init` | Write default config for OS |

**Flags:** `--mode fast|smart|code|auto` · `--model` · `--dry-run` · `--yes` · `--no-shell` · `--context` · `--verbose` · `--provider-url`

---

## 7. Tool system (agent contract)

| Tool | Behavior |
|------|----------|
| `list_files` | Repo files with ignore rules |
| `read_file` | Bounded line ranges |
| `search_text` | ripgrep-ranked matches |
| `git_status` / `git_diff` | Branch + changes |
| `apply_patch` | Validate → preview → approve → apply |
| `run_tests` | Known test runners only |
| `run_command` | Argv-only; policy gated |

**Agent loop:** select one model → chat → tool calls (validate) → append results → until final answer or `maxSteps`.

### Command policy

| Class | Examples | Policy |
|-------|----------|--------|
| Auto-safe | `rg`, `git status/diff`, `go test`, `pytest`, `npm test` | Allow in-repo |
| Approval | install, docker, checkout, migrate | Ask user |
| Blocked | `sudo`, `rm -rf`, `mkfs`, `git push --force` | Reject by default |

### Secrets deny-list (never inject into context)

`.env*`, `*.pem`, `*.key`, `id_rsa`, `id_ed25519`, `.aws/**`, `.ssh/**`, `**/credentials*`, `**/secrets*`

---

## 8. Routing heuristics (auto)

```text
if explanation/simple AND ≤2 files     → fast
else if implement/refactor/fix/code   → code
else if architecture/plan/review/deep  → smart
else                                   → fast
```

Escalate at most **once** per request if the first model fails tool-following badly — never thrash 27B↔27B every step.

---

## 9. Safety & editing model

1. Model proposes **unified diff** only  
2. Validate paths (no `..`, no absolute escape, no `.git`)  
3. Symlink-resolve before write  
4. Show `git`-style preview  
5. User approve (unless `--yes`)  
6. Apply → optional formatter → tests → capture post-diff  

Undo: store pre-patch blobs under session `patches/` + `git checkout --` when clean enough.

---

## 10. Build roadmap (milestones)

| ID | Deliverable | Done when |
|----|-------------|-----------|
| **M0** | Ollama + 3 models (Linux first) | Smoke tests + GPU/RAM notes |
| **M1** | Go CLI chat/ask + modes | Streams from all 3 models |
| **M2** | Repo read tools | Answers without paste |
| **M3** | Git awareness | `review` works |
| **M4** | Patch apply + approval | Safe edits |
| **M5** | Tests + shell policy | Fix loop can run tests |
| **M6** | Full agent loop | `fix`/`edit` iterate to limit |
| **M7** | Auto router | Correct pick + override |
| **M8** | Context budget + summaries | Large repos usable |
| **M9** | Tree-sitter symbols | Better cross-file nav |
| **M10** | Cross-platform packaging | Linux/Windows/macOS releases |
| **M11** | TUI polish + docs | Daily-driver UX |
| **M12** | (Optional) remote/iOS companion | Phone → home server |

**Implementation order (exact):**

1. Ollama install + model storage + one-model policy  
2. Pull three models + benchmark identical tasks  
3. Go module + config + Ollama stream client  
4. `--mode` selection  
5. `git` root + `ls-files`  
6. Read-only tools (`read_file`, `search_text`)  
7. `git_status` / `git_diff`  
8. Tool-call agent loop + step limit  
9. Patch validate/preview/apply  
10. Tests + command policy  
11. `fix` / `edit` workflows  
12. Auto router  
13. History compression  
14. Cross-compile CI + installers  
15. Tree-sitter (only after retrieval is proven)

---

## 11. Testing strategy

**Unit:** path sandbox, policy classifier, diff parser, token budget, router heuristics, stream/tool JSON decode  

**Integration:** temp Git repos + mocked Ollama; assert no out-of-repo writes; assert “tests passed” ⇒ exit 0  

**Live (opt-in):** `LOCALCODE_LIVE=1` against real Ollama  

**Benchmark suite:** fixtures with expected test outcomes; score all three modes after prompt/router changes  

---

## 12. Distribution plan

| Channel | Artifact |
|---------|----------|
| GitHub Releases | OS/arch binaries + SHA256 |
| Homebrew | `brew install localcode` (macOS/Linux) |
| WinGet | Windows |
| Scoop (optional) | Windows power users |
| apt/yum (later) | Linux packages |
| Docker (optional) | CLI + Ollama sidecar for demos |

Install script:

```bash
curl -fsSL https://example.com/install.sh | sh   # Linux/macOS
# Windows: winget install LocalCode.LocalCode
```

---

## 13. Hardware profiles (product docs)

Document three profiles so users know what to expect:

| Profile | Hardware | Recommendation |
|---------|----------|----------------|
| **Workstation (you)** | 32 GB + 8 GB VRAM | All 3 models; one at a time; 8–16K context |
| **Laptop mid** | 16 GB RAM, weak/no GPU | Prefer `fast` only; pull 9B; optional remote 27B |
| **Server** | 64 GB+ / multi-GPU | Can raise context; still prefer one heavy model loaded |

Remote mode: laptop CLI → `provider.base_url: http://home-server:11434` (same binary, different config).

---

## 14. Success criteria

You can `cd` into any Git repo on **Linux, Windows, or macOS**, run `localcode`, ask for a feature or bugfix, review a proposed diff, run tests locally, and finish **without any cloud model**.

---

## 15. Decision summary (lock these)

| Decision | Choice |
|----------|--------|
| Product name (working) | `localcode` |
| CLI language | Go |
| Model host | Ollama (abstracted) |
| Models | 9B fast / 27B smart / 27B coding |
| Edit model | Unified diff + approval |
| Platforms v1 | Linux + Windows + macOS |
| iOS | Remote SSH / Phase-2 companion only |
| Memory policy | One model loaded |
| Retrieval v1 | Git + ripgrep (Tree-sitter later) |

---

## 16. Next action (when you say go)

1. Scaffold the Go module + Cobra commands  
2. Wire Ollama streaming + `ask`/`chat`  
3. Run M0 model pulls on your Linux box if not done  

Say **“start M1”** (or M0) and implementation begins from the skeleton.
