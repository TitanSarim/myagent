package tools

import (
	"path/filepath"

	"github.com/TitanSarim/myagent/internal/config"
	"github.com/TitanSarim/myagent/internal/repo"
)

type Options struct {
	Repo     *repo.Repo
	Cfg      *config.Config
	Approve  Approver
	Writable bool
	DryRun   bool
	NoShell  bool
	Session  string
}

// NewReadOnly builds the M2/M3 tool set (no writes / no shell).
func NewReadOnly(r *repo.Repo) *Registry {
	return NewRegistry(
		&ListFiles{Repo: r},
		&ReadFile{Repo: r},
		&SearchText{Repo: r},
		&GitStatus{Repo: r},
		&GitDiff{Repo: r},
	)
}

// NewAgentTools builds read tools plus optional write/shell tools.
func NewAgentTools(opt Options) *Registry {
	ts := []Tool{
		&ListFiles{Repo: opt.Repo},
		&ReadFile{Repo: opt.Repo},
		&SearchText{Repo: opt.Repo},
		&GitStatus{Repo: opt.Repo},
		&GitDiff{Repo: opt.Repo},
	}
	if opt.Writable {
		ts = append(ts, &ApplyPatch{
			Repo:    opt.Repo,
			Cfg:     opt.Cfg,
			Approve: opt.Approve,
			DryRun:  opt.DryRun,
			Session: opt.Session,
		})
		if !opt.NoShell {
			ts = append(ts,
				&RunTests{Repo: opt.Repo, Cfg: opt.Cfg, Approve: opt.Approve, NoShell: opt.NoShell},
				&RunCommand{Repo: opt.Repo, Cfg: opt.Cfg, Approve: opt.Approve, NoShell: opt.NoShell},
			)
		}
	}
	return NewRegistry(ts...)
}

func SessionPatchDir(dataDir, sessionID string) string {
	if dataDir == "" || sessionID == "" {
		return ""
	}
	return filepath.Join(dataDir, "sessions", sessionID, "patches")
}
