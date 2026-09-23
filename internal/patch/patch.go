package patch

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FilePatch is one file section of a unified diff.
type FilePatch struct {
	OldPath string // a/ path (relative, no a/ prefix stored)
	NewPath string // b/ path
	IsNew   bool
	IsDelete bool
	Hunks   []Hunk
	Raw     string
}

type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []string // includes leading ' ', '+', '-', '\'
}

type Set struct {
	Files []FilePatch
}

// ParseUnified parses a unified diff that may contain multiple files.
func ParseUnified(diff string) (*Set, error) {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return nil, fmt.Errorf("empty patch")
	}

	// Allow fenced code blocks from models.
	if strings.HasPrefix(diff, "```") {
		lines := strings.Split(diff, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if n := len(lines); n > 0 && strings.HasPrefix(lines[n-1], "```") {
				lines = lines[:n-1]
			}
			diff = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}

	sc := bufio.NewScanner(strings.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var set Set
	var cur *FilePatch
	var hunk *Hunk
	var raw bytes.Buffer

	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
			hunk = nil
		}
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			cur.Raw = raw.String()
			set.Files = append(set.Files, *cur)
			cur = nil
			raw.Reset()
		}
	}

	for sc.Scan() {
		line := sc.Text()

		if strings.HasPrefix(line, "diff --git ") {
			flushFile()
			cur = &FilePatch{}
			raw.WriteString(line)
			raw.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(line, "--- ") {
			if cur == nil {
				cur = &FilePatch{}
			}
			raw.WriteString(line)
			raw.WriteByte('\n')
			p := strings.TrimPrefix(line, "--- ")
			p = strings.TrimSpace(strings.SplitN(p, "\t", 2)[0])
			if p == "/dev/null" {
				cur.IsNew = true
				cur.OldPath = ""
			} else {
				cur.OldPath = stripAB(p)
			}
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			if cur == nil {
				cur = &FilePatch{}
			}
			raw.WriteString(line)
			raw.WriteByte('\n')
			p := strings.TrimPrefix(line, "+++ ")
			p = strings.TrimSpace(strings.SplitN(p, "\t", 2)[0])
			if p == "/dev/null" {
				cur.IsDelete = true
				cur.NewPath = ""
			} else {
				cur.NewPath = stripAB(p)
			}
			continue
		}
		if strings.HasPrefix(line, "@@ ") {
			flushHunk()
			if cur == nil {
				return nil, fmt.Errorf("hunk without file header")
			}
			raw.WriteString(line)
			raw.WriteByte('\n')
			h, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			hunk = &h
			continue
		}
		if hunk != nil {
			if len(line) == 0 {
				// empty line treated as context with missing marker
				hunk.Lines = append(hunk.Lines, " ")
				raw.WriteByte('\n')
				continue
			}
			switch line[0] {
			case ' ', '+', '-', '\\':
				hunk.Lines = append(hunk.Lines, line)
				raw.WriteString(line)
				raw.WriteByte('\n')
			default:
				// end of hunk material
				flushHunk()
			}
			continue
		}
		if cur != nil {
			raw.WriteString(line)
			raw.WriteByte('\n')
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	flushFile()

	if len(set.Files) == 0 {
		return nil, fmt.Errorf("no file hunks found in patch")
	}
	for i := range set.Files {
		f := &set.Files[i]
		if f.NewPath == "" && !f.IsDelete {
			return nil, fmt.Errorf("file %d missing +++ path", i)
		}
		if len(f.Hunks) == 0 && !f.IsNew && !f.IsDelete {
			return nil, fmt.Errorf("file %s has no hunks", f.Path())
		}
	}
	return &set, nil
}

func (f FilePatch) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

func stripAB(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

func parseHunkHeader(line string) (Hunk, error) {
	// @@ -l,s +l,s @@ optional
	var h Hunk
	var oldStart, oldCount, newStart, newCount int
	oldCount, newCount = 1, 1
	rest := strings.TrimPrefix(line, "@@ ")
	parts := strings.SplitN(rest, "@@", 2)
	if len(parts) < 1 {
		return h, fmt.Errorf("bad hunk header: %s", line)
	}
	ranges := strings.Fields(parts[0])
	if len(ranges) < 2 {
		return h, fmt.Errorf("bad hunk ranges: %s", line)
	}
	if err := parseRange(ranges[0], &oldStart, &oldCount); err != nil {
		return h, err
	}
	if err := parseRange(ranges[1], &newStart, &newCount); err != nil {
		return h, err
	}
	if strings.HasPrefix(ranges[0], "+") {
		// unusual order — reject
		return h, fmt.Errorf("unexpected hunk order: %s", line)
	}
	h.OldStart, h.OldCount = oldStart, oldCount
	h.NewStart, h.NewCount = newStart, newCount
	return h, nil
}

func parseRange(s string, start, count *int) error {
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "+")
	if strings.Contains(s, ",") {
		_, err := fmt.Sscanf(s, "%d,%d", start, count)
		return err
	}
	_, err := fmt.Sscanf(s, "%d", start)
	*count = 1
	return err
}

// ValidatePaths ensures all paths stay inside repo root and are not protected.
func (s *Set) ValidatePaths(repoRoot string, absFn func(string) (string, error)) error {
	protected := []string{".git", ".localcode"}
	for _, f := range s.Files {
		for _, p := range []string{f.OldPath, f.NewPath} {
			if p == "" {
				continue
			}
			if filepath.IsAbs(p) {
				return fmt.Errorf("absolute paths not allowed: %s", p)
			}
			clean := filepath.Clean(p)
			if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("path escapes repo: %s", p)
			}
			for _, bad := range protected {
				if clean == bad || strings.HasPrefix(clean, bad+string(os.PathSeparator)) {
					return fmt.Errorf("protected path: %s", p)
				}
			}
			if absFn != nil {
				if _, err := absFn(clean); err != nil {
					return err
				}
			}
		}
	}
	_ = repoRoot
	return nil
}

// Apply applies the patch set to files under repoRoot.
func (s *Set) Apply(repoRoot string) error {
	for _, f := range s.Files {
		if err := applyFile(repoRoot, f); err != nil {
			return fmt.Errorf("%s: %w", f.Path(), err)
		}
	}
	return nil
}

func applyFile(root string, f FilePatch) error {
	path := f.Path()
	abs := filepath.Join(root, filepath.FromSlash(path))

	if f.IsDelete {
		return os.Remove(abs)
	}

	var oldLines []string
	if !f.IsNew {
		data, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		oldLines = splitKeep(string(data))
	}

	newLines, err := applyHunks(oldLines, f.Hunks)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	out := strings.Join(newLines, "\n")
	// Preserve trailing newline if original had one or file is new/nonempty
	if len(newLines) > 0 {
		out += "\n"
	}
	return os.WriteFile(abs, []byte(out), 0o644)
}

func splitKeep(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}

func applyHunks(old []string, hunks []Hunk) ([]string, error) {
	// Work on a copy; apply in order using old line numbers adjusted by delta.
	out := append([]string{}, old...)
	delta := 0
	for _, h := range hunks {
		start := h.OldStart - 1 + delta
		if start < 0 {
			start = 0
		}
		var remove int
		var add []string
		for _, line := range h.Lines {
			if line == "\\ No newline at end of file" || strings.HasPrefix(line, "\\") {
				continue
			}
			if len(line) == 0 {
				continue
			}
			switch line[0] {
			case ' ':
				add = append(add, line[1:])
				remove++
			case '-':
				remove++
			case '+':
				add = append(add, line[1:])
			}
		}
		// Verify context/removals match when possible
		if start+remove > len(out) && remove > 0 {
			return nil, fmt.Errorf("hunk at old line %d overflows file (%d lines)", h.OldStart, len(out))
		}
		for i := 0; i < remove; i++ {
			wantIdx := -1
			// Find expected old content from hunk '-' and ' ' lines
			_ = wantIdx
		}
		// Soft apply: check removed lines match
		expected := expectedOld(h)
		if len(expected) > 0 {
			actual := out[start : start+len(expected)]
			if !linesEqual(actual, expected) {
				// try fuzzy: search nearby
				found := -1
				window := 40
				from := start - window
				if from < 0 {
					from = 0
				}
				to := start + window
				if to > len(out)-len(expected) {
					to = len(out) - len(expected)
				}
				for i := from; i <= to; i++ {
					if i < 0 || i+len(expected) > len(out) {
						continue
					}
					if linesEqual(out[i:i+len(expected)], expected) {
						found = i
						break
					}
				}
				if found < 0 {
					return nil, fmt.Errorf("context mismatch at line %d", h.OldStart)
				}
				start = found
			}
		}

		end := start + remove
		replaced := append([]string{}, out[:start]...)
		replaced = append(replaced, add...)
		replaced = append(replaced, out[end:]...)
		delta += len(add) - remove
		out = replaced
	}
	return out, nil
}

func expectedOld(h Hunk) []string {
	var out []string
	for _, line := range h.Lines {
		if len(line) == 0 || line[0] == '\\' {
			continue
		}
		switch line[0] {
		case ' ', '-':
			out = append(out, line[1:])
		}
	}
	return out
}

func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Summary returns a short human description.
func (s *Set) Summary() string {
	var b strings.Builder
	for _, f := range s.Files {
		kind := "modify"
		if f.IsNew {
			kind = "create"
		} else if f.IsDelete {
			kind = "delete"
		}
		fmt.Fprintf(&b, "%s %s (%d hunks)\n", kind, f.Path(), len(f.Hunks))
	}
	return b.String()
}

// Preview returns the normalized patch text for display.
func (s *Set) Preview() string {
	var b strings.Builder
	for _, f := range s.Files {
		if f.Raw != "" {
			b.WriteString(f.Raw)
			if !strings.HasSuffix(f.Raw, "\n") {
				b.WriteByte('\n')
			}
			continue
		}
		old := f.OldPath
		newp := f.NewPath
		if f.IsNew {
			old = "/dev/null"
		} else {
			old = "a/" + old
		}
		if f.IsDelete {
			newp = "/dev/null"
		} else {
			newp = "b/" + newp
		}
		fmt.Fprintf(&b, "--- %s\n+++ %s\n", old, newp)
		for _, h := range f.Hunks {
			fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
			for _, line := range h.Lines {
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}
	}
	return b.String()
}

// WriteDiffFile writes patch text to w.
func WriteDiffFile(w io.Writer, s *Set) error {
	_, err := io.WriteString(w, s.Preview())
	return err
}
