package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/TitanSarim/myagent/internal/agent"
	"github.com/TitanSarim/myagent/internal/config"
	"github.com/TitanSarim/myagent/internal/provider"
	"github.com/TitanSarim/myagent/internal/repo"
	"github.com/TitanSarim/myagent/internal/router"
	"github.com/TitanSarim/myagent/internal/session"
	"github.com/TitanSarim/myagent/internal/tools"
	"github.com/TitanSarim/myagent/internal/tui"
	"github.com/TitanSarim/myagent/internal/ui"
	"github.com/spf13/cobra"
)

var (
	version = "0.4.0-m8"
	// appName is the installed command name (overridable via LOCALCODE_NAME).
	appName = "localcode"

	flagMode    string
	flagModel   string
	flagContext int
	flagVerbose bool
	flagURL     string
	flagNoTools bool
	flagYes     bool
	flagDryRun  bool
	flagNoShell bool
	flagWrite   bool
)

func appDisplayName() string {
	if n := os.Getenv("LOCALCODE_NAME"); n != "" {
		return n
	}
	return appName
}

func main() {
	if n := os.Getenv("LOCALCODE_NAME"); n != "" {
		appName = n
	}
	root := &cobra.Command{
		Use:   appName,
		Short: "Local AI coding CLI (Ollama + Qwen)",
		Long:  appName + " — local coding agent. Run with no args to open the terminal UI.",
		RunE:  runChat,
	}

	root.PersistentFlags().StringVar(&flagMode, "mode", "", "Model mode: fast|smart|code|auto")
	root.PersistentFlags().StringVar(&flagModel, "model", "", "Override Ollama model name")
	root.PersistentFlags().IntVar(&flagContext, "context", 0, "Override context window (num_ctx)")
	root.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")
	root.PersistentFlags().StringVar(&flagURL, "provider-url", "", "Override Ollama base URL")
	root.PersistentFlags().BoolVar(&flagNoTools, "no-tools", false, "Disable repository tools")
	root.PersistentFlags().BoolVar(&flagYes, "yes", false, "Auto-approve patches and approval-gated commands")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Validate patches but do not write files")
	root.PersistentFlags().BoolVar(&flagNoShell, "no-shell", false, "Disable run_command/run_tests")
	root.PersistentFlags().BoolVar(&flagWrite, "write", false, "Enable write tools in chat/ask (edit/fix enable this by default)")

	root.AddCommand(
		newAskCmd(),
		newChatCmd(),
		newExplainCmd(),
		newPlanCmd(),
		newEditCmd(),
		newFixCmd(),
		newReviewCmd(),
		newTestCmd(),
		newModelsCmd(),
		newStatusCmd(),
		newInitCmd(),
		newVersionCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

type runtimeEnv struct {
	cfg      *config.Config
	prov     provider.Provider
	router   *router.Router
	repo     *repo.Repo
	tools    *tools.Registry
	session  *session.Store
	writable bool
}

func loadRuntime(writable bool) (*runtimeEnv, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if flagURL != "" {
		cfg.Provider.BaseURL = flagURL
	}
	p := provider.NewOllama(cfg.Provider.BaseURL)
	r := router.New(cfg)

	env := &runtimeEnv{cfg: cfg, prov: p, router: r, writable: writable}

	dataDir, _ := config.DataDir()
	if dataDir != "" {
		if store, err := session.New(dataDir); err == nil {
			env.session = store
		}
	}

	if flagNoTools {
		return env, nil
	}
	rp, err := repo.Find("")
	if err != nil {
		return env, nil
	}
	env.repo = rp

	approve := tools.DefaultApprover(flagYes)
	patchDir := ""
	if env.session != nil {
		patchDir = env.session.PatchDir()
	}

	if writable {
		env.tools = tools.NewAgentTools(tools.Options{
			Repo:     rp,
			Cfg:      cfg,
			Approve:  approve,
			Writable: true,
			DryRun:   flagDryRun,
			NoShell:  flagNoShell,
			Session:  patchDir,
		})
	} else {
		env.tools = tools.NewReadOnly(rp)
	}
	return env, nil
}

func (e *runtimeEnv) agent() *agent.Agent {
	return &agent.Agent{
		Provider: e.prov,
		Router:   e.router,
		Config:   e.cfg,
		Repo:     e.repo,
		Tools:    e.tools,
		Session:  e.session,
		Verbose:  flagVerbose,
		Writable: e.writable,
		DryRun:   flagDryRun,
	}
}

func modeOrDefault(cfg *config.Config) string {
	if flagMode != "" {
		return flagMode
	}
	return cfg.Routing.Default
}

func runAgent(cmd *cobra.Command, prompt, mode string, writable bool) error {
	env, err := loadRuntime(writable)
	if err != nil {
		return err
	}
	if err := env.prov.Ping(cmd.Context()); err != nil {
		return err
	}
	if mode == "" {
		mode = modeOrDefault(env.cfg)
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	_, err = env.agent().Run(ctx, agent.RunOptions{
		Prompt:  prompt,
		Mode:    mode,
		Model:   flagModel,
		Context: flagContext,
	})
	return err
}

func newAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ask [prompt...]",
		Short: "One-shot question (repo tools; add --write to allow patches)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(cmd, strings.Join(args, " "), "", flagWrite)
		},
	}
}

func newExplainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain [path]",
		Short: "Explain a file, directory, or the repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			prompt := fmt.Sprintf(
				"Explain %q in this repository. Use tools to inspect it. Cover purpose, key parts, and how it fits the project. Be concise.",
				target,
			)
			mode := modeOrDefault(mustCfg())
			if flagMode == "" {
				mode = "fast"
			}
			return runAgent(cmd, prompt, mode, false)
		},
	}
}

func newPlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "plan [prompt...]",
		Short: "Create an implementation plan without changing files",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := "Create a concrete implementation plan (no code edits). Inspect the repo with tools first.\n\nTask: " + strings.Join(args, " ")
			mode := "smart"
			if flagMode != "" {
				mode = flagMode
			}
			return runAgent(cmd, prompt, mode, false)
		},
	}
}

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit [prompt...]",
		Short: "Inspect repo and apply a reviewed patch",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := "Implement the following change. Inspect relevant files with tools, then apply_patch with a minimal unified diff. Summarize files changed.\n\nTask: " + strings.Join(args, " ")
			mode := "code"
			if flagMode != "" {
				mode = flagMode
			}
			return runAgent(cmd, prompt, mode, true)
		},
	}
}

func newFixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fix [prompt...]",
		Short: "Investigate, patch, test, and iterate",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := "Fix the problem described below. Use tools to diagnose, apply_patch to edit, then run_tests. Iterate until tests pass or you are blocked. Summarize what changed.\n\nProblem: " + strings.Join(args, " ")
			mode := "code"
			if flagMode != "" {
				mode = flagMode
			}
			return runAgent(cmd, prompt, mode, true)
		},
	}
}

func newReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review",
		Short: "Review the current git diff",
		RunE: func(cmd *cobra.Command, _ []string) error {
			prompt := "Review the current git working tree changes. Use git_status and git_diff. Report bugs, risks, and missing tests. Be concrete."
			mode := "smart"
			if flagMode != "" {
				mode = flagMode
			}
			return runAgent(cmd, prompt, mode, false)
		},
	}
}

func newTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test [target]",
		Short: "Run tests and summarize failures (agent-assisted)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			prompt := "Run the test suite using run_tests"
			if target != "" {
				prompt += fmt.Sprintf(" with target %q", target)
			}
			prompt += ". Summarize failures and likely causes. Do not edit files unless asked."
			mode := "fast"
			if flagMode != "" {
				mode = flagMode
			}
			return runAgent(cmd, prompt, mode, true)
		},
	}
}

func mustCfg() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		return config.Default()
	}
	return cfg
}

func newChatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Interactive terminal UI",
		RunE:  runChat,
	}
}

func runChat(cmd *cobra.Command, _ []string) error {
	return runTerminalUI(cmd)
}

func runTerminalUI(cmd *cobra.Command) error {
	writable := flagWrite
	env, err := loadRuntime(writable)
	if err != nil {
		return err
	}
	if err := env.prov.Ping(cmd.Context()); err != nil {
		return fmt.Errorf("%w\nIs Ollama running? Try: ollama serve", err)
	}

	repoLabel := "(none)"
	if env.repo != nil {
		repoLabel = env.repo.Name()
	}
	mode := modeOrDefault(env.cfg)

	toolNames := []string{}
	if env.tools != nil {
		for _, d := range env.tools.Definitions() {
			toolNames = append(toolNames, d.Name)
		}
	}

	history := ""

	return tui.RunInteractive(tui.Config{
		AppName:  appDisplayName(),
		Version:  version,
		Repo:     repoLabel,
		Mode:     mode,
		Writable: writable,
		Tools:    toolNames,
		OnSlash: func(c string) (bool, string, string, *bool) {
			lower := strings.ToLower(strings.TrimSpace(c))
			switch {
			case lower == "/status":
				var b strings.Builder
				fmt.Fprintf(&b, "app: %s %s\n", appDisplayName(), version)
				fmt.Fprintf(&b, "repo: %s\n", repoLabel)
				fmt.Fprintf(&b, "mode: %s\n", mode)
				fmt.Fprintf(&b, "writes: %v\n", writable)
				if env.session != nil {
					fmt.Fprintf(&b, "session: %s\n", env.session.Dir)
				}
				if err := env.prov.Ping(context.Background()); err != nil {
					fmt.Fprintf(&b, "ollama: unreachable (%v)\n", err)
				} else {
					b.WriteString("ollama: ok\n")
				}
				return true, b.String(), "", nil
			case lower == "/tools":
				if env.tools == nil {
					return true, "No tools (not in a git repo, or --no-tools).", "", nil
				}
				var b strings.Builder
				b.WriteString("Available tools:\n")
				for _, d := range env.tools.Definitions() {
					fmt.Fprintf(&b, "  • %s — %s\n", d.Name, d.Description)
				}
				return true, b.String(), "", nil
			case strings.HasPrefix(lower, "/mode "):
				mode = strings.TrimSpace(c[6:])
				return true, "Mode set to " + mode, mode, nil
			case lower == "/write on":
				flagWrite = true
				writable = true
				env, err = loadRuntime(true)
				if err != nil {
					return true, "error: " + err.Error(), "", nil
				}
				toolNames = toolNames[:0]
				if env.tools != nil {
					for _, d := range env.tools.Definitions() {
						toolNames = append(toolNames, d.Name)
					}
				}
				w := true
				return true, "Writes enabled. Patches may ask for approval (or use --yes).", "", &w
			case lower == "/write off":
				flagWrite = false
				writable = false
				env, err = loadRuntime(false)
				if err != nil {
					return true, "error: " + err.Error(), "", nil
				}
				w := false
				return true, "Writes disabled (read-only).", "", &w
			case lower == "/clear":
				history = ""
				return true, "History cleared.", "", nil
			}
			return false, "", "", nil
		},
		OnSubmit: func(ctx context.Context, text string, emit tui.Emitter) error {
			prompt := text
			if history != "" {
				prompt = history + "\n\nUser: " + text
			}
			ag := env.agent()
			if emit != nil {
				ag.OnInfo = emit.Info
				ag.OnToken = emit.Token
			}
			answer, err := ag.Run(ctx, agent.RunOptions{
				Prompt:  prompt,
				Mode:    mode,
				Model:   flagModel,
				Context: flagContext,
			})
			if err != nil {
				return err
			}
			history = trimHistory(history + "\nUser: " + text + "\nAssistant: " + answer)
			return nil
		},
	})
}

func trimHistory(s string) string {
	const max = 12_000
	if len(s) <= max {
		return s
	}
	return "...[earlier conversation truncated]...\n" + s[len(s)-max:]
}

func newModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "Show model mode mappings and Ollama availability",
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := loadRuntime(false)
			if err != nil {
				return err
			}
			fmt.Println("Configured modes:")
			printMode("fast", env.cfg.Models.Fast)
			printMode("smart", env.cfg.Models.Smart)
			printMode("code", env.cfg.Models.Code)
			fmt.Println()

			if err := env.prov.Ping(cmd.Context()); err != nil {
				return fmt.Errorf("ollama: %w", err)
			}
			models, err := env.prov.ListModels(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Printf("Ollama models at %s:\n", env.cfg.Provider.BaseURL)
			have := map[string]bool{}
			for _, m := range models {
				have[m.Name] = true
				fmt.Printf("  %-28s  %8.1f GB  %s\n", m.Name, float64(m.Size)/1e9, m.Family)
			}
			fmt.Println()
			for _, pair := range []struct{ mode, name string }{
				{"fast", env.cfg.Models.Fast.Name},
				{"smart", env.cfg.Models.Smart.Name},
				{"code", env.cfg.Models.Code.Name},
			} {
				status := "MISSING"
				if have[pair.name] {
					status = "ok"
				}
				fmt.Printf("  %s → %s [%s]\n", pair.mode, pair.name, status)
			}
			return nil
		},
	}
}

func printMode(name string, m config.ModelProfile) {
	fmt.Printf("  %-6s  %-22s  ctx=%d  keep_alive=%s\n", name, m.Name, m.Context, m.KeepAlive)
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show config, provider, repo, and runtime status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := loadRuntime(flagWrite)
			if err != nil {
				return err
			}
			printStatus(env, modeOrDefault(env.cfg))
			return nil
		},
	}
}

func printStatus(env *runtimeEnv, mode string) {
	cfgPath, _ := config.Path()
	dataDir, _ := config.DataDir()
	fmt.Printf("os:           %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("config:       %s\n", cfgPath)
	fmt.Printf("data:         %s\n", dataDir)
	fmt.Printf("provider:     %s (%s)\n", env.cfg.Provider.Type, env.cfg.Provider.BaseURL)
	fmt.Printf("default mode: %s\n", env.cfg.Routing.Default)
	fmt.Printf("active mode:  %s\n", mode)
	fmt.Printf("writes:       %v (dry-run=%v yes=%v)\n", env.writable, flagDryRun, flagYes)
	if env.session != nil {
		fmt.Printf("session:      %s\n", env.session.Dir)
	}
	if env.repo != nil {
		fmt.Printf("repo:         %s\n", env.repo.Root)
		if env.tools != nil {
			fmt.Printf("tools:        %d enabled\n", len(env.tools.Definitions()))
		}
	} else {
		fmt.Println("repo:         (none)")
		fmt.Println("tools:        disabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := env.prov.Ping(ctx); err != nil {
		fmt.Printf("ollama:       unreachable (%v)\n", err)
		return
	}
	fmt.Println("ollama:       ok")
	models, err := env.prov.ListModels(ctx)
	if err != nil {
		fmt.Printf("models:       error (%v)\n", err)
		return
	}
	fmt.Printf("models:       %d available\n", len(models))
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write default config for this OS",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.WriteDefault()
			if err != nil {
				return err
			}
			ui.Infof("wrote %s", path)
			return nil
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Println(appName, version)
		},
	}
}
