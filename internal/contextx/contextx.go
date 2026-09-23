package contextx

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/TitanSarim/myagent/internal/provider"
	"github.com/TitanSarim/myagent/internal/repo"
)

// RoughCharsPerToken approximates token count for budgeting.
const RoughCharsPerToken = 4

func EstimateTokens(s string) int {
	n := utf8.RuneCountInString(s)
	if n == 0 {
		return 0
	}
	t := n / RoughCharsPerToken
	if t < 1 {
		return 1
	}
	return t
}

func EstimateMessages(msgs []provider.Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateTokens(m.Content) + 4
		for _, tc := range m.ToolCalls {
			total += EstimateTokens(tc.Name) + 8
			for k, v := range tc.Arguments {
				total += EstimateTokens(k) + EstimateTokens(fmt.Sprint(v))
			}
		}
	}
	return total
}

// TrimMessages keeps system + recent turns under budget.
// Older non-system messages are replaced by a summary stub.
func TrimMessages(msgs []provider.Message, budgetTokens int) []provider.Message {
	if budgetTokens <= 0 || EstimateMessages(msgs) <= budgetTokens {
		return msgs
	}
	if len(msgs) == 0 {
		return msgs
	}

	var system []provider.Message
	var rest []provider.Message
	for _, m := range msgs {
		if m.Role == provider.RoleSystem {
			system = append(system, m)
		} else {
			rest = append(rest, m)
		}
	}

	// Keep newest messages until budget fits.
	kept := make([]provider.Message, 0, len(rest))
	used := EstimateMessages(system)
	for i := len(rest) - 1; i >= 0; i-- {
		cost := EstimateTokens(rest[i].Content) + 4
		if used+cost > budgetTokens && len(kept) > 0 {
			break
		}
		kept = append([]provider.Message{rest[i]}, kept...)
		used += cost
	}

	dropped := len(rest) - len(kept)
	out := append([]provider.Message{}, system...)
	if dropped > 0 {
		out = append(out, provider.Message{
			Role: provider.RoleSystem,
			Content: fmt.Sprintf(
				"[context trimmed: %d earlier messages omitted to stay within ~%d token budget]",
				dropped, budgetTokens,
			),
		})
	}
	return append(out, kept...)
}

// RankedFile is a repo path with a relevance score.
type RankedFile struct {
	Path  string
	Score int
}

// RankFiles scores tracked files against query tokens (lexical).
func RankFiles(files []string, query string, limit int) []RankedFile {
	if limit <= 0 {
		limit = 12
	}
	tokens := tokenize(query)
	if len(tokens) == 0 {
		return nil
	}
	var ranked []RankedFile
	for _, f := range files {
		lower := strings.ToLower(filepath.ToSlash(f))
		base := strings.ToLower(filepath.Base(f))
		score := 0
		for _, t := range tokens {
			if strings.Contains(base, t) {
				score += 5
			}
			if strings.Contains(lower, t) {
				score += 2
			}
		}
		// Prefer source-like files
		switch filepath.Ext(lower) {
		case ".go", ".ts", ".tsx", ".js", ".py", ".rs", ".java", ".md":
			score += 1
		}
		if score > 0 {
			ranked = append(ranked, RankedFile{Path: f, Score: score})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].Path < ranked[j].Path
		}
		return ranked[i].Score > ranked[j].Score
	})
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return ranked
}

func tokenize(q string) []string {
	q = strings.ToLower(q)
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '/')
	})
	stop := map[string]bool{
		"a": true, "an": true, "the": true, "to": true, "and": true, "or": true,
		"of": true, "in": true, "on": true, "for": true, "with": true, "is": true,
		"this": true, "that": true, "please": true, "me": true, "my": true,
		"use": true, "using": true, "from": true, "into": true,
	}
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		if len(f) < 2 || stop[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// BuildRepoBrief creates a compact repo primer for the system prompt.
func BuildRepoBrief(r *repo.Repo, query string, maxFiles int) string {
	if r == nil {
		return ""
	}
	files, err := r.ListFiles("", 3000)
	if err != nil || len(files) == 0 {
		return ""
	}
	ranked := RankFiles(files, query, maxFiles)
	var b strings.Builder
	fmt.Fprintf(&b, "Repository has ~%d tracked/untracked files.\n", len(files))
	if status, err := r.Status(); err == nil && strings.TrimSpace(status) != "" {
		lines := strings.Split(strings.TrimSpace(status), "\n")
		if len(lines) > 8 {
			lines = lines[:8]
		}
		b.WriteString("Git status (truncated):\n")
		for _, ln := range lines {
			b.WriteString("  ")
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	if len(ranked) > 0 {
		b.WriteString("Likely relevant files for this request:\n")
		for _, rf := range ranked {
			fmt.Fprintf(&b, "  - %s (score %d)\n", rf.Path, rf.Score)
		}
		b.WriteString("Prefer reading these first with tools.\n")
	}
	return b.String()
}

// SummarizeActions builds a short rolling summary from tool result snippets.
func SummarizeActions(actions []string, maxTokens int) string {
	if len(actions) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Recent actions:\n")
	for i, a := range actions {
		line := fmt.Sprintf("%d. %s\n", i+1, oneLine(a, 160))
		if EstimateTokens(b.String()+line) > maxTokens {
			b.WriteString("…\n")
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func oneLine(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
