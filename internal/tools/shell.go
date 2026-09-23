package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/TitanSarim/myagent/internal/config"
	"github.com/TitanSarim/myagent/internal/repo"
	"github.com/TitanSarim/myagent/internal/security"
	"github.com/TitanSarim/myagent/internal/ui"
)

type RunCommand struct {
	Repo    *repo.Repo
	Cfg     *config.Config
	Approve Approver
	NoShell bool
}

func (t *RunCommand) Def() Definition {
	return Definition{
		Name: "run_command",
		Description: "Run a policy-checked command as argv (no shell interpolation). " +
			"Example argv: [\"go\",\"test\",\"./...\"]",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"argv": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Command and arguments",
				},
				"reason": map[string]any{
					"type":        "string",
					"description": "Why this command is needed",
				},
			},
			"required": []string{"argv"},
		},
	}
}

func (t *RunCommand) Run(ctx context.Context, args map[string]any) (string, error) {
	if t.NoShell {
		return "", fmt.Errorf("commands disabled (--no-shell)")
	}
	argv, err := argStringSlice(args, "argv")
	if err != nil {
		return "", err
	}
	if len(argv) == 0 {
		return "", fmt.Errorf("argv is required")
	}
	reason := argString(args, "reason")

	class := security.ClassifyCommand(argv)
	switch class {
	case security.ClassBlocked:
		return "", fmt.Errorf("blocked command: %v", argv)
	case security.ClassApproval:
		need := true
		if t.Cfg != nil {
			need = t.Cfg.Safety.RequireDangerousCommandApproval
		}
		if need {
			body := fmt.Sprintf("Command: %v\nClass: %s\nReason: %s", argv, class, reason)
			ok := true
			var aerr error
			if t.Approve != nil {
				ok, aerr = t.Approve("run_command", body)
			}
			if aerr != nil {
				return "", aerr
			}
			if !ok {
				return "Command rejected by user.", nil
			}
		}
	case security.ClassAuto:
		ui.Infof("  (auto-safe) %v", argv)
	}

	return execCapture(ctx, t.Repo.Root, argv, 120*time.Second)
}

type RunTests struct {
	Repo    *repo.Repo
	Cfg     *config.Config
	Approve Approver
	NoShell bool
}

func (t *RunTests) Def() Definition {
	return Definition{
		Name: "run_tests",
		Description: "Run the repository test suite. Detects go/npm/pytest. " +
			"Optional target selects a package/path (e.g. ./internal/patch).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "Optional test target/path",
				},
			},
		},
	}
}

func (t *RunTests) Run(ctx context.Context, args map[string]any) (string, error) {
	if t.NoShell {
		return "", fmt.Errorf("commands disabled (--no-shell)")
	}
	target := argString(args, "target")
	argv, err := detectTestArgv(t.Repo.Root, target)
	if err != nil {
		return "", err
	}
	ui.Infof("  (tests) %v", argv)
	return execCapture(ctx, t.Repo.Root, argv, 180*time.Second)
}

func detectTestArgv(root, target string) ([]string, error) {
	if fileExists(filepath.Join(root, "go.mod")) {
		if target == "" {
			target = "./..."
		}
		return []string{"go", "test", target}, nil
	}
	if fileExists(filepath.Join(root, "package.json")) {
		return []string{"npm", "test", "--silent"}, nil
	}
	if fileExists(filepath.Join(root, "pytest.ini")) ||
		fileExists(filepath.Join(root, "pyproject.toml")) ||
		fileExists(filepath.Join(root, "requirements.txt")) {
		if target == "" {
			return []string{"pytest", "-q"}, nil
		}
		return []string{"pytest", "-q", target}, nil
	}
	return nil, fmt.Errorf("could not detect test runner (no go.mod/package.json/pytest)")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func execCapture(ctx context.Context, dir string, argv []string, timeout time.Duration) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String() + stderr.String()
	if len(out) > 60_000 {
		out = out[:60_000] + "\n...[truncated]"
	}
	if err != nil {
		return fmt.Sprintf("exit_error: %v\n%s", err, out), nil
	}
	if strings.TrimSpace(out) == "" {
		out = "(no output)\n"
	}
	return "exit_ok\n" + out, nil
}

func argStringSlice(args map[string]any, key string) ([]string, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, fmt.Errorf("%s is required", key)
	}
	switch t := v.(type) {
	case []string:
		return t, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			out = append(out, fmt.Sprint(x))
		}
		return out, nil
	case string:
		fields := strings.Fields(t)
		if len(fields) == 0 {
			return nil, fmt.Errorf("%s is empty", key)
		}
		return fields, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
}
