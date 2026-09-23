package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TitanSarim/myagent/internal/repo"
)

func TestReadFileAndSearch(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "git", "init")
	mustRun(t, dir, "git", "config", "user.email", "test@example.com")
	mustRun(t, dir, "git", "config", "user.name", "test")
	src := filepath.Join(dir, "hello.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc Hello() string { return \"hi\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "git", "add", ".")
	mustRun(t, dir, "git", "commit", "-m", "init")

	r, err := repo.Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	reg := NewReadOnly(r)

	out, err := reg.tools["read_file"].Run(context.Background(), map[string]any{"path": "hello.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "func Hello") {
		t.Fatalf("unexpected read: %s", out)
	}

	sout, err := reg.tools["search_text"].Run(context.Background(), map[string]any{"query": "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sout, "hello.go") {
		t.Fatalf("unexpected search: %s", sout)
	}
}

func mustRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := newCmd(dir, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
