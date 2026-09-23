package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TitanSarim/myagent/internal/config"
	"github.com/TitanSarim/myagent/internal/patch"
	"github.com/TitanSarim/myagent/internal/repo"
	"github.com/TitanSarim/myagent/internal/ui"
)

type Approver func(title, body string) (bool, error)

func DefaultApprover(autoYes bool) Approver {
	return func(title, body string) (bool, error) {
		if autoYes {
			ui.Infof("auto-approved: %s", title)
			return true, nil
		}
		ui.Infof("")
		ui.Infof("=== %s ===", title)
		fmt.Fprintln(ui.ErrOut, body)
		ui.Infof("Apply this change? [y/N]")
		sc := bufio.NewScanner(os.Stdin)
		if !sc.Scan() {
			return false, sc.Err()
		}
		ans := strings.TrimSpace(strings.ToLower(sc.Text()))
		return ans == "y" || ans == "yes", nil
	}
}

type ApplyPatch struct {
	Repo     *repo.Repo
	Cfg      *config.Config
	Approve  Approver
	DryRun   bool
	Session  string // optional dir to save patches
}

func (t *ApplyPatch) Def() Definition {
	return Definition{
		Name: "apply_patch",
		Description: "Validate and apply a unified diff patch inside the repository. " +
			"Pass the full unified diff in `diff`. Prefer minimal hunks. User approval may be required.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"diff": map[string]any{
					"type":        "string",
					"description": "Unified diff text (---/+++/@@ hunks)",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Short reason for the change",
				},
			},
			"required": []string{"diff"},
		},
	}
}

func (t *ApplyPatch) Run(_ context.Context, args map[string]any) (string, error) {
	diffText := argString(args, "diff")
	reason := argString(args, "reason")
	if diffText == "" {
		return "", fmt.Errorf("diff is required")
	}

	set, err := patch.ParseUnified(diffText)
	if err != nil {
		return "", fmt.Errorf("parse patch: %w", err)
	}
	if err := set.ValidatePaths(t.Repo.Root, t.Repo.Abs); err != nil {
		return "", fmt.Errorf("unsafe patch: %w", err)
	}

	preview := set.Preview()
	summary := set.Summary()
	body := summary + "\n" + preview
	if reason != "" {
		body = "Reason: " + reason + "\n\n" + body
	}

	needApproval := true
	if t.Cfg != nil {
		needApproval = t.Cfg.Safety.RequirePatchApproval
	}
	if t.DryRun {
		return "DRY-RUN — patch validated, not applied:\n" + body, nil
	}

	if needApproval {
		ok := true
		var err error
		if t.Approve != nil {
			ok, err = t.Approve("apply_patch", body)
		}
		if err != nil {
			return "", err
		}
		if !ok {
			return "Patch rejected by user.", nil
		}
	}

	if t.Session != "" {
		_ = os.MkdirAll(t.Session, 0o750)
		name := fmt.Sprintf("patch-%d.diff", time.Now().Unix())
		_ = os.WriteFile(filepath.Join(t.Session, name), []byte(preview), 0o640)
	}

	if err := set.Apply(t.Repo.Root); err != nil {
		return "", fmt.Errorf("apply failed: %w", err)
	}
	return "Patch applied:\n" + summary, nil
}
