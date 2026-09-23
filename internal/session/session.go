package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Store struct {
	Dir string
}

func New(dataDir string) (*Store, error) {
	id := time.Now().Format("20060102-150405")
	dir := filepath.Join(dataDir, "sessions", id)
	if err := os.MkdirAll(filepath.Join(dir, "patches"), 0o750); err != nil {
		return nil, err
	}
	meta := map[string]any{
		"id":         id,
		"started_at": time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "meta.json"), b, 0o640)
	return &Store{Dir: dir}, nil
}

func (s *Store) AppendJSONL(name string, v any) error {
	if s == nil {
		return nil
	}
	path := filepath.Join(s.Dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(v)
}

func (s *Store) LogAction(kind, detail string) {
	if s == nil {
		return
	}
	_ = s.AppendJSONL("actions.jsonl", map[string]any{
		"ts":     time.Now().UTC().Format(time.RFC3339),
		"kind":   kind,
		"detail": trim(detail, 2000),
	})
}

func (s *Store) LogMessage(role, content string) {
	if s == nil {
		return
	}
	_ = s.AppendJSONL("conversation.jsonl", map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339),
		"role":    role,
		"content": trim(content, 8000),
	})
}

func (s *Store) PatchDir() string {
	if s == nil {
		return ""
	}
	return filepath.Join(s.Dir, "patches")
}

func (s *Store) WriteSummary(text string) error {
	if s == nil {
		return nil
	}
	return os.WriteFile(filepath.Join(s.Dir, "summary.md"), []byte(text), 0o640)
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("…(%d more bytes)", len(s)-n)
}
