package router

import (
	"fmt"
	"strings"

	"github.com/TitanSarim/myagent/internal/config"
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

// Signals are optional repo/runtime hints for auto routing.
type Signals struct {
	ChangedFiles int
	FailingTests int
	RepoFiles    int
}

type Router struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Router {
	return &Router{cfg: cfg}
}

func (r *Router) Select(mode string, prompt string) (Decision, error) {
	return r.SelectWith(mode, prompt, Signals{})
}

func (r *Router) SelectWith(mode string, prompt string, sig Signals) (Decision, error) {
	m := Mode(strings.ToLower(strings.TrimSpace(mode)))
	if m == "" {
		m = Mode(r.cfg.Routing.Default)
	}
	if m == ModeAuto {
		chosen, why := heuristic(prompt, sig)
		profile, err := r.cfg.Profile(string(chosen))
		if err != nil {
			return Decision{}, err
		}
		return Decision{Mode: chosen, Profile: profile, Reason: "auto: " + why}, nil
	}
	profile, err := r.cfg.Profile(string(m))
	if err != nil {
		return Decision{}, err
	}
	return Decision{Mode: m, Profile: profile, Reason: "explicit"}, nil
}

func heuristic(prompt string, sig Signals) (Mode, string) {
	p := strings.ToLower(prompt)

	smartHints := []string{
		"architect", "design", "trade-off", "tradeoff", "plan ", "planning",
		"diagnose", "root cause", "review", "migrate", "migration",
		"complex", "ambiguous", "why does", "how should we",
	}
	codeHints := []string{
		"implement", "add ", "fix", "refactor", "create", "write",
		"patch", "feature", "bug", "failing", "edit", "change",
		"update the", "multi-file", "apply",
	}

	codeScore, smartScore := 0, 0
	for _, h := range codeHints {
		if strings.Contains(p, h) {
			codeScore++
		}
	}
	for _, h := range smartHints {
		if strings.Contains(p, h) {
			smartScore++
		}
	}

	// Repo signals
	if sig.FailingTests > 0 {
		codeScore += 2
	}
	if sig.ChangedFiles >= 5 {
		codeScore++
	}
	if sig.ChangedFiles >= 15 || sig.RepoFiles > 2000 {
		smartScore++
	}

	switch {
	case codeScore > 0 && codeScore >= smartScore:
		return ModeCode, fmt.Sprintf("code signals=%d smart=%d changed=%d failing=%d", codeScore, smartScore, sig.ChangedFiles, sig.FailingTests)
	case smartScore > 0:
		return ModeSmart, fmt.Sprintf("smart signals=%d code=%d", smartScore, codeScore)
	default:
		return ModeFast, "default fast"
	}
}
