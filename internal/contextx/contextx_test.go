package contextx

import (
	"strings"
	"testing"

	"github.com/TitanSarim/myagent/internal/provider"
)

func TestEstimateAndTrim(t *testing.T) {
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "system rules"},
	}
	for i := 0; i < 40; i++ {
		msgs = append(msgs, provider.Message{
			Role:    provider.RoleUser,
			Content: "user message with enough padding content to consume budget tokens XXXXXXXXXXX",
		})
		msgs = append(msgs, provider.Message{
			Role:    provider.RoleAssistant,
			Content: "assistant reply with enough padding content to consume budget tokens YYYYYYYYYYY",
		})
	}
	before := EstimateMessages(msgs)
	trimmed := TrimMessages(msgs, 120)
	after := EstimateMessages(trimmed)
	if after >= before {
		t.Fatalf("expected trim to reduce tokens %d -> %d", before, after)
	}
	found := false
	for _, m := range trimmed {
		if strings.Contains(m.Content, "context trimmed") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected trim note")
	}
}

func TestRankFiles(t *testing.T) {
	files := []string{
		"internal/router/router.go",
		"internal/ui/ui.go",
		"README.md",
		"cmd/localcode/main.go",
	}
	ranked := RankFiles(files, "fix router heuristic", 5)
	if len(ranked) == 0 {
		t.Fatal("expected ranks")
	}
	if ranked[0].Path != "internal/router/router.go" {
		t.Fatalf("expected router first, got %s", ranked[0].Path)
	}
}
