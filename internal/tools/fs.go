package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/sarim/localcode/internal/repo"
)

type ListFiles struct{ Repo *repo.Repo }

func (t *ListFiles) Def() Definition {
	return Definition{
		Name:        "list_files",
		Description: "List tracked and untracked (non-ignored) files in the repository. Prefer a subdirectory when possible.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Optional subdirectory relative to repo root",
				},
				"max": map[string]any{
					"type":        "integer",
					"description": "Maximum files to return (default 200)",
				},
			},
		},
	}
}

func (t *ListFiles) Run(_ context.Context, args map[string]any) (string, error) {
	path := argString(args, "path")
	max := argInt(args, "max", 200)
	files, err := t.Repo.ListFiles(path, max)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "(no files)", nil
	}
	var b strings.Builder
	for _, f := range files {
		b.WriteString(f)
		b.WriteByte('\n')
	}
	if len(files) >= max {
		fmt.Fprintf(&b, "... truncated at %d files\n", max)
	}
	return b.String(), nil
}

type ReadFile struct{ Repo *repo.Repo }

func (t *ReadFile) Def() Definition {
	return Definition{
		Name:        "read_file",
		Description: "Read a text file from the repository. Use start_line/end_line for large files (1-based, inclusive).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path relative to repository root",
				},
				"start_line": map[string]any{"type": "integer", "description": "First line (1-based)"},
				"end_line":   map[string]any{"type": "integer", "description": "Last line (1-based, inclusive)"},
			},
			"required": []string{"path"},
		},
	}
}

func (t *ReadFile) Run(_ context.Context, args map[string]any) (string, error) {
	path := argString(args, "path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if isSecretPath(path) {
		return "", fmt.Errorf("refusing to read secret-like path: %s", path)
	}
	abs, err := t.Repo.Abs(path)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", fmt.Errorf("%s is a directory; use list_files", path)
	}
	if st.Size() > 2_000_000 {
		return "", fmt.Errorf("file too large (%d bytes); request a line range", st.Size())
	}

	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()

	start := argInt(args, "start_line", 1)
	end := argInt(args, "end_line", 0)
	if start < 1 {
		start = 1
	}

	var b strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	emitted := 0
	const maxLines = 400
	const maxBytes = 120_000

	for sc.Scan() {
		lineNo++
		if lineNo < start {
			continue
		}
		if end > 0 && lineNo > end {
			break
		}
		line := sc.Text()
		if !utf8.ValidString(line) {
			return "", fmt.Errorf("binary or non-utf8 file: %s", path)
		}
		fmt.Fprintf(&b, "%6d|%s\n", lineNo, line)
		emitted++
		if emitted >= maxLines || b.Len() >= maxBytes {
			fmt.Fprintf(&b, "... truncated after line %d\n", lineNo)
			break
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if emitted == 0 {
		return fmt.Sprintf("(empty or no lines in range for %s)", path), nil
	}
	return b.String(), nil
}

type GitStatus struct{ Repo *repo.Repo }

func (t *GitStatus) Def() Definition {
	return Definition{
		Name:        "git_status",
		Description: "Show current branch and short git status for the repository.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (t *GitStatus) Run(_ context.Context, _ map[string]any) (string, error) {
	return t.Repo.Status()
}

type GitDiff struct{ Repo *repo.Repo }

func (t *GitDiff) Def() Definition {
	return Definition{
		Name:        "git_diff",
		Description: "Show git diff of working tree (or staged changes if staged=true).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"staged": map[string]any{
					"type":        "boolean",
					"description": "If true, show staged diff only",
				},
			},
		},
	}
}

func (t *GitDiff) Run(_ context.Context, args map[string]any) (string, error) {
	return t.Repo.Diff(argBool(args, "staged", false))
}

func isSecretPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	p := strings.ToLower(filepath.ToSlash(path))
	secrets := []string{
		".env", ".pem", ".key", "id_rsa", "id_ed25519", "credentials", "secrets",
	}
	for _, s := range secrets {
		if base == s || strings.HasSuffix(base, s) || strings.Contains(p, "/"+s) {
			return true
		}
	}
	if strings.HasPrefix(base, ".env") {
		return true
	}
	if strings.Contains(p, "/.ssh/") || strings.Contains(p, "/.aws/") {
		return true
	}
	return false
}
