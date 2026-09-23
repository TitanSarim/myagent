package tools

import "github.com/sarim/localcode/internal/repo"

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
