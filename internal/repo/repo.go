package repo

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Repo struct {
	Root string
}

// Find walks up from start looking for a .git directory.
func Find(start string) (*Repo, error) {
	if start == "" {
		var err error
		start, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	dir := abs
	for {
		gitDir := filepath.Join(dir, ".git")
		if st, err := os.Stat(gitDir); err == nil && (st.IsDir() || st.Mode().IsRegular()) {
			return &Repo{Root: dir}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fallback: git rev-parse
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = abs
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("not a git repository (from %s)", abs)
	}
	root := strings.TrimSpace(string(out))
	return &Repo{Root: root}, nil
}

func (r *Repo) Rel(path string) (string, error) {
	abs, err := r.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(r.Root, abs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository: %s", path)
	}
	return filepath.ToSlash(rel), nil
}

func (r *Repo) Abs(path string) (string, error) {
	if path == "" {
		return r.Root, nil
	}
	var abs string
	if filepath.IsAbs(path) {
		abs = filepath.Clean(path)
	} else {
		abs = filepath.Clean(filepath.Join(r.Root, path))
	}
	// Resolve symlinks for sandbox checks when possible.
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	}
	rootResolved, err := filepath.EvalSymlinks(r.Root)
	if err != nil {
		rootResolved = r.Root
	}
	rel, err := filepath.Rel(rootResolved, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path outside repository: %s", path)
	}
	return abs, nil
}

func (r *Repo) ListFiles(subdir string, max int) ([]string, error) {
	if max <= 0 {
		max = 2000
	}
	args := []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard"}
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	prefix := ""
	if subdir != "" {
		rel, err := r.Rel(subdir)
		if err != nil {
			return nil, err
		}
		if rel != "." {
			prefix = rel
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
		}
	}
	parts := bytes.Split(out, []byte{0})
	files := make([]string, 0, 256)
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		s := filepath.ToSlash(string(p))
		if prefix != "" && !strings.HasPrefix(s, prefix) {
			continue
		}
		files = append(files, s)
		if len(files) >= max {
			break
		}
	}
	return files, nil
}

func (r *Repo) Name() string {
	return filepath.Base(r.Root)
}

func (r *Repo) Git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

func (r *Repo) Status() (string, error) {
	branch, _ := r.Git("rev-parse", "--abbrev-ref", "HEAD")
	status, err := r.Git("status", "--short", "--branch")
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(status)
	if b := strings.TrimSpace(branch); b != "" && !strings.Contains(out, b) {
		out = "## " + b + "\n" + out
	}
	return out, nil
}

func (r *Repo) Diff(staged bool) (string, error) {
	args := []string{"diff", "--no-color"}
	if staged {
		args = append(args, "--cached")
	}
	out, err := r.Git(args...)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) == "" {
		return "(no changes)", nil
	}
	// Cap huge diffs
	const max = 80_000
	if len(out) > max {
		return out[:max] + "\n... [diff truncated]", nil
	}
	return out, nil
}
