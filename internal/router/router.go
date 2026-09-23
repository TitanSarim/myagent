package router

import (
	"strings"

	"github.com/sarim/localcode/internal/config"
)

type Mode string

const (
	ModeFast  Mode = "fast"
	ModeSmart Mode = "smart"
	ModeCode  Mode = "code"
	ModeAuto  Mode = "auto"
)

type Decision struct {
	Mode    Mode
	Profile config.ModelProfile
	Reason  string
}

type Router struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Router {
	return &Router{cfg: cfg}
}

func (r *Router) Select(mode string, prompt string) (Decision, error) {
	m := Mode(strings.ToLower(strings.TrimSpace(mode)))
	if m == "" {
		m = Mode(r.cfg.Routing.Default)
	}
	if m == ModeAuto {
		m = heuristic(prompt)
		profile, err := r.cfg.Profile(string(m))
		if err != nil {
			return Decision{}, err
		}
		return Decision{Mode: m, Profile: profile, Reason: "auto heuristic"}, nil
	}
	profile, err := r.cfg.Profile(string(m))
	if err != nil {
		return Decision{}, err
	}
	return Decision{Mode: m, Profile: profile, Reason: "explicit"}, nil
}

func heuristic(prompt string) Mode {
	p := strings.ToLower(prompt)

	smartHints := []string{
		"architect", "design", "trade-off", "tradeoff", "plan ", "planning",
		"why", "diagnose", "root cause", "review", "migrate", "migration",
		"complex", "ambiguous",
	}
	codeHints := []string{
		"implement", "add ", "fix", "refactor", "create", "write",
		"patch", "feature", "bug", "test", "failing", "edit", "change",
		"update the", "multi-file",
	}

	for _, h := range smartHints {
		if strings.Contains(p, h) {
			// Prefer code when it also looks like an implementation ask.
			for _, c := range codeHints {
				if strings.Contains(p, c) && (strings.Contains(p, "implement") ||
					strings.Contains(p, "fix") ||
					strings.Contains(p, "refactor") ||
					strings.Contains(p, "add ")) {
					return ModeCode
				}
			}
			return ModeSmart
		}
	}
	for _, h := range codeHints {
		if strings.Contains(p, h) {
			return ModeCode
		}
	}
	return ModeFast
}
