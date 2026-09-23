package tools

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sarim/localcode/internal/repo"
)

type SearchText struct{ Repo *repo.Repo }

func (t *SearchText) Def() Definition {
	return Definition{
		Name:        "search_text",
		Description: "Search repository text with ripgrep (or a Go fallback). Returns ranked matching lines.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search pattern (literal preferred)",
				},
				"glob": map[string]any{
					"type":        "string",
					"description": "Optional glob filter, e.g. *.go",
				},
				"max": map[string]any{
					"type":        "integer",
					"description": "Max matches (default 40)",
				},
			},
			"required": []string{"query"},
		},
	}
}

func (t *SearchText) Run(ctx context.Context, args map[string]any) (string, error) {
	query := argString(args, "query")
	if query == "" {
		return "", fmt.Errorf("query is required")
	}
	glob := argString(args, "glob")
	max := argInt(args, "max", 40)
	if max <= 0 {
		max = 40
	}

	if rg := findRipgrep(); rg != "" {
		out, err := t.ripgrep(ctx, rg, query, glob, max)
		if err == nil {
			return out, nil
		}
		// fall through on rg errors other than no-match
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "(no matches)", nil
		}
	}
	return t.fallbackSearch(query, glob, max)
}

func (t *SearchText) ripgrep(ctx context.Context, rg, query, glob string, max int) (string, error) {
	args := []string{
		"--line-number",
		"--no-heading",
		"--color", "never",
		"--hidden",
		"--glob", "!.git/**",
		"--glob", "!vendor/**",
		"--glob", "!node_modules/**",
		"--glob", "!bin/**",
		"--max-count", fmt.Sprintf("%d", max),
		"--max-filesize", "1M",
	}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, "--", query, ".")

	cmd := exec.CommandContext(ctx, rg, args...)
	cmd.Dir = t.Repo.Root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "(no matches)", nil
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("rg: %s", msg)
		}
		return "", err
	}
	out := stdout.String()
	if strings.TrimSpace(out) == "" {
		return "(no matches)", nil
	}
	lines := strings.Split(out, "\n")
	if len(lines) > max {
		lines = lines[:max]
		out = strings.Join(lines, "\n") + "\n... truncated\n"
	}
	return out, nil
}

func (t *SearchText) fallbackSearch(query, glob string, max int) (string, error) {
	files, err := t.Repo.ListFiles("", 5000)
	if err != nil {
		return "", err
	}
	q := strings.ToLower(query)
	var b strings.Builder
	matches := 0
	for _, rel := range files {
		if glob != "" {
			ok, _ := filepath.Match(glob, filepath.Base(rel))
			if !ok {
				ok, _ = filepath.Match(glob, rel)
			}
			if !ok {
				continue
			}
		}
		if isSecretPath(rel) {
			continue
		}
		abs, err := t.Repo.Abs(rel)
		if err != nil {
			continue
		}
		f, err := os.Open(abs)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			if strings.Contains(strings.ToLower(line), q) {
				fmt.Fprintf(&b, "%s:%d:%s\n", rel, lineNo, trimRunes(line, 200))
				matches++
				if matches >= max {
					f.Close()
					b.WriteString("... truncated\n")
					return b.String(), nil
				}
			}
		}
		f.Close()
	}
	if matches == 0 {
		return "(no matches)", nil
	}
	return b.String(), nil
}

func findRipgrep() string {
	if p, err := exec.LookPath("rg"); err == nil {
		return p
	}
	candidates := []string{
		"/usr/bin/rg",
		"/usr/local/bin/rg",
		"/usr/share/cursor/resources/app/node_modules/@vscode/ripgrep/bin/rg",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func trimRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
