package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/TitanSarim/myagent/internal/config"
	"github.com/TitanSarim/myagent/internal/contextx"
	"github.com/TitanSarim/myagent/internal/provider"
	"github.com/TitanSarim/myagent/internal/repo"
	"github.com/TitanSarim/myagent/internal/router"
	"github.com/TitanSarim/myagent/internal/session"
	"github.com/TitanSarim/myagent/internal/tools"
	"github.com/TitanSarim/myagent/internal/ui"
	"github.com/TitanSarim/myagent/prompts"
)

const DefaultMaxSteps = 16

type Agent struct {
	Provider provider.Provider
	Router   *router.Router
	Config   *config.Config
	Repo     *repo.Repo
	Tools    *tools.Registry
	Session  *session.Store
	MaxSteps int
	Verbose  bool
	Writable bool
	DryRun   bool

	// Optional UI hooks (TUI). When set, replace default stdout/stderr printing.
	OnInfo  func(string)
	OnToken func(string)
}

func (a *Agent) info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if a.OnInfo != nil {
		a.OnInfo(msg)
		return
	}
	ui.Infof("%s", msg)
}

func (a *Agent) token(s string) {
	if a.OnToken != nil {
		a.OnToken(s)
		return
	}
	ui.StreamWrite(s)
}

func (a *Agent) newline() {
	if a.OnToken != nil {
		a.OnToken("\n")
		return
	}
	ui.Newline()
}

type RunOptions struct {
	Prompt  string
	Mode    string
	Model   string
	Context int
}

func (a *Agent) systemPrompt(userPrompt string) string {
	base := strings.TrimSpace(prompts.System)
	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\n")
	if a.Repo != nil {
		fmt.Fprintf(&b, "Repository root: %s\n", a.Repo.Root)
		fmt.Fprintf(&b, "Repository name: %s\n", a.Repo.Name())
		b.WriteString("Use tools before guessing about files.\n")
		if a.Tools != nil {
			names := make([]string, 0, len(a.Tools.Definitions()))
			for _, d := range a.Tools.Definitions() {
				names = append(names, d.Name)
			}
			fmt.Fprintf(&b, "Available tools: %s.\n", strings.Join(names, ", "))
		}
		if brief := contextx.BuildRepoBrief(a.Repo, userPrompt, 10); brief != "" {
			b.WriteString("\n")
			b.WriteString(brief)
		}
		if a.Writable {
			b.WriteString("You MAY edit files using apply_patch with a valid unified diff.\n")
			b.WriteString("After editing, prefer run_tests when appropriate.\n")
			b.WriteString("Never claim a patch applied unless apply_patch succeeded.\n")
			if a.DryRun {
				b.WriteString("DRY-RUN is active: patches will be validated but not written.\n")
			}
		} else {
			b.WriteString("This session is read-only: do not claim you edited files.\n")
		}
	} else {
		b.WriteString("No git repository detected in the current directory.\n")
	}
	return b.String()
}

func (a *Agent) repoSignals() router.Signals {
	sig := router.Signals{}
	if a.Repo == nil {
		return sig
	}
	if files, err := a.Repo.ListFiles("", 5000); err == nil {
		sig.RepoFiles = len(files)
	}
	if st, err := a.Repo.Status(); err == nil {
		for _, ln := range strings.Split(st, "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" || strings.HasPrefix(ln, "##") {
				continue
			}
			sig.ChangedFiles++
		}
	}
	return sig
}

func (a *Agent) tokenBudget(ctxTokens int) int {
	// Reserve headroom for model reply + tool schemas.
	if ctxTokens <= 0 {
		ctxTokens = 8192
	}
	budget := (ctxTokens * 60) / 100
	if budget < 1500 {
		budget = 1500
	}
	return budget
}

func (a *Agent) Run(ctx context.Context, opt RunOptions) (string, error) {
	if a.MaxSteps <= 0 {
		a.MaxSteps = DefaultMaxSteps
	}

	sig := a.repoSignals()
	d, err := a.Router.SelectWith(opt.Mode, opt.Prompt, sig)
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
	a.info("mode:%s  model:%s  (%s)", d.Mode, d.Profile.Name, d.Reason)
	a.info("repo: %s", repoName)
	if a.Session != nil {
		a.info("session: %s", a.Session.Dir)
	}
	if a.Writable {
		if a.DryRun {
			a.info("writes: dry-run")
		} else {
			a.info("writes: enabled")
		}
	}
	if a.Verbose {
		a.info("context budget ~%d tokens (num_ctx=%d)", a.tokenBudget(d.Profile.Context), d.Profile.Context)
	}

	if a.Session != nil {
		a.Session.LogMessage("user", opt.Prompt)
	}

	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: a.systemPrompt(opt.Prompt)},
		{Role: provider.RoleUser, Content: opt.Prompt},
	}

	var toolDefs []map[string]any
	if a.Tools != nil {
		toolDefs = a.Tools.OllamaTools()
	}

	budget := a.tokenBudget(d.Profile.Context)
	var final strings.Builder
	var actions []string

	for step := 0; step < a.MaxSteps; step++ {
		if a.Verbose {
			a.info("agent step %d/%d (msgs≈%d tokens)", step+1, a.MaxSteps, contextx.EstimateMessages(messages))
		}

		messages = contextx.TrimMessages(messages, budget)

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
				a.token(ev.Content)
				content.WriteString(ev.Content)
			case provider.EventToolCalls:
				toolCalls = ev.ToolCalls
				if ev.Content != "" && content.Len() == 0 {
					content.WriteString(ev.Content)
				}
			case provider.EventError:
				a.newline()
				return final.String(), ev.Err
			case provider.EventDone:
			}
		}

		if len(toolCalls) == 0 {
			if content.Len() > 0 {
				a.newline()
				final.WriteString(content.String())
				if a.Session != nil {
					a.Session.LogMessage("assistant", content.String())
					_ = a.Session.WriteSummary(content.String())
				}
			}
			return final.String(), nil
		}

		messages = append(messages, provider.Message{
			Role:      provider.RoleAssistant,
			Content:   content.String(),
			ToolCalls: toolCalls,
		})

		for _, tc := range toolCalls {
			a.info("→ %s %s", tc.Name, compactArgs(tc.Arguments))
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
				a.info("  ✗ %s", preview)
			} else {
				a.info("  ✓ %s", oneLine(preview))
			}
			actions = append(actions, tc.Name+": "+oneLine(res.Content))
			if a.Session != nil {
				a.Session.LogAction(tc.Name, res.Content)
			}
			// Cap huge tool payloads in the message stream.
			toolContent := res.Content
			if contextx.EstimateTokens(toolContent) > 2500 {
				toolContent = toolContent[:10000] + "\n...[tool output truncated for context budget]"
			}
			messages = append(messages, provider.Message{
				Role:       provider.RoleTool,
				Content:    toolContent,
				ToolName:   tc.Name,
				ToolCallID: tc.ID,
			})
		}

		if sum := contextx.SummarizeActions(actions, 400); sum != "" && step%3 == 2 {
			messages = append(messages, provider.Message{
				Role:    provider.RoleSystem,
				Content: sum,
			})
		}
	}

	a.newline()
	return final.String(), fmt.Errorf("agent step limit reached (%d)", a.MaxSteps)
}

func compactArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		if k == "diff" {
			parts = append(parts, "diff=<unified diff>")
			continue
		}
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
