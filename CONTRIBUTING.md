# Contributing to myagent

Thanks for helping make this open source!

## Dev setup

```bash
git clone https://github.com/TitanSarim/myagent.git
cd myagent
go test ./...
go build -o bin/myagent ./cmd/localcode
./scripts/install.sh myagent
```

## Guidelines

- Keep the CLI local-first (Ollama / OpenAI-compatible providers).
- Prefer small, reviewable PRs.
- Add tests for patch/security/router changes.
- Do not commit secrets, model weights, or `bin/` / `dist/` binaries.

## License

By contributing, you agree your contributions are licensed under the MIT License.
