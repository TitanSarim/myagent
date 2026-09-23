package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sarim/localcode/internal/config"
	"github.com/sarim/localcode/internal/provider"
	"github.com/sarim/localcode/internal/repo"
	"github.com/sarim/localcode/internal/router"
	"github.com/sarim/localcode/internal/tools"
	"github.com/sarim/localcode/internal/ui"
	"github.com/sarim/localcode/prompts"
)

const DefaultMaxSteps = 12

type Agent struct {
	Provider provider.Provider
	Router   *router.Router
	Config   *config.Config
	Repo     *repo.Repo
	Tools    *tools.Registry
	MaxSteps int
	Verbose  bool
}

type RunOptions struct {
	Prompt  string
	Mode    string
	Model   string
	Context int
}

func (a *Agent) systemPrompt() string {
	base := strings.TrimSpace(prompts.System)
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\n")
	if a.Repo != nil {
		fmt.Fprintf(&b, "Repository root: %s\n", a.Repo.Root)
		fmt.Fprintf(&b, "Repository name: %s\n", a.Repo.Name())
		b.WriteString("You have tools to inspect this repository. Use tools before guessing about files.\n")
		b.WriteString("Available tools: list_files, read_file, search_text, git_status, git_diff.\n")
		b.WriteString("This session is read-only: do not claim you edited files.\n")
	} else {
		b.WriteString("No git repository detected in the current directory.\n")
	}
	return b.String()
}

func (a *Agent) Run(ctx context.Context, opt RunOptions) (string, error) {
	if a.MaxSteps <= 0 {
		a.MaxSteps = DefaultMaxSteps
	}

	d, err := a.Router.Select(opt.Mode, opt.Prompt)
	if err != nil {
		return "", err
	}
	if opt.Model != "" {
		d.Profile.Name = opt.Model
		d.Reason = "model override"
	}
	if opt.Context > 0 {
		d.Profile.Context = opt.Context
	}

	repoName := "(none)"
	if a.Repo != nil {
		repoName = a.Repo.Name()
	}
	ui.Header(string(d.Mode), d.Profile.Name, d.Reason)
	ui.Infof("repo: %s", repoName)

	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: a.systemPrompt()},
		{Role: provider.RoleUser, Content: opt.Prompt},
	}

	var toolDefs []map[string]any
	if a.Tools != nil {
		toolDefs = a.Tools.OllamaTools()
	}

	var final strings.Builder

	for step := 0; step < a.MaxSteps; step++ {
		if a.Verbose {
			ui.Infof("agent step %d/%d", step+1, a.MaxSteps)
		}

		ch, err := a.Provider.Chat(ctx, provider.ChatRequest{
			Model:     d.Profile.Name,
			Messages:  messages,
			Tools:     toolDefs,
			Context:   d.Profile.Context,
			KeepAlive: d.Profile.KeepAlive,
			Stream:    true,
		})
		if err != nil {
			return final.String(), err
		}

		var content strings.Builder
		var toolCalls []provider.ToolCall

		for ev := range ch {
			switch ev.Kind {
			case provider.EventToken:
				ui.StreamWrite(ev.Content)
				content.WriteString(ev.Content)
			case provider.EventToolCalls:
				toolCalls = ev.ToolCalls
				if ev.Content != "" && content.Len() == 0 {
					content.WriteString(ev.Content)
				}
			case provider.EventError:
				ui.Newline()
				return final.String(), ev.Err
			case provider.EventDone:
				// continue after loop
			}
		}

		if len(toolCalls) == 0 {
			if content.Len() > 0 {
				ui.Newline()
				final.WriteString(content.String())
			}
			return final.String(), nil
		}

		// Record assistant tool call turn
		messages = append(messages, provider.Message{
			Role:      provider.RoleAssistant,
			Content:   content.String(),
			ToolCalls: toolCalls,
		})

		for _, tc := range toolCalls {
			ui.Infof("→ %s %s", tc.Name, compactArgs(tc.Arguments))
			res := a.Tools.Run(ctx, tools.Call{
				ID:        tc.ID,
				Name:      tc.Name,
				Arguments: tc.Arguments,
			})
			preview := res.Content
			if len(preview) > 240 {
				preview = preview[:240] + "…"
			}
			if res.IsError {
				ui.Infof("  ✗ %s", preview)
			} else {
				ui.Infof("  ✓ %s", oneLine(preview))
			}
			messages = append(messages, provider.Message{
				Role:       provider.RoleTool,
				Content:    res.Content,
				ToolName:   tc.Name,
				ToolCallID: tc.ID,
			})
		}
	}

	ui.Newline()
	return final.String(), fmt.Errorf("agent step limit reached (%d)", a.MaxSteps)
}

func compactArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	s := strings.Join(parts, " ")
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}
