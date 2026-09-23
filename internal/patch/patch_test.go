package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndApplyModify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diff := `--- a/hello.txt
+++ b/hello.txt
@@ -1,1 +1,1 @@
-hello world
+hello localcode
`
	set, err := ParseUnified(diff)
	if err != nil {
		t.Fatal(err)
	}
	if err := set.ValidatePaths(dir, func(p string) (string, error) {
		return filepath.Join(dir, p), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := set.Apply(dir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.TrimSpace(string(data)) != "hello localcode" {
		t.Fatalf("got %q", data)
	}
}

func TestRejectEscape(t *testing.T) {
	diff := `--- a/../etc/passwd
+++ b/../etc/passwd
@@ -1 +1 @@
-a
+b
`
	set, err := ParseUnified(diff)
	if err != nil {
		t.Fatal(err)
	}
	err = set.ValidatePaths("/tmp/repo", nil)
	if err == nil {
		t.Fatal("expected path escape error")
	}
}

func TestParseNewFile(t *testing.T) {
	dir := t.TempDir()
	diff := `--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+one
+two
`
	set, err := ParseUnified(diff)
	if err != nil {
		t.Fatal(err)
	}
	if !set.Files[0].IsNew {
		t.Fatal("expected new file")
	}
	if err := set.Apply(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "one") {
		t.Fatalf("got %q", data)
	}
}
