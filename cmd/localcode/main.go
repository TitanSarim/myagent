package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/sarim/localcode/internal/agent"
	"github.com/sarim/localcode/internal/config"
	"github.com/sarim/localcode/internal/provider"
	"github.com/sarim/localcode/internal/repo"
	"github.com/sarim/localcode/internal/router"
	"github.com/sarim/localcode/internal/tools"
	"github.com/sarim/localcode/internal/ui"
	"github.com/spf13/cobra"
)

var (
	flagMode    string
	flagModel   string
	flagContext int
	flagVerbose bool
	flagURL     string
	flagNoTools bool
)

func main() {
	root := &cobra.Command{
		Use:   "localcode",
		Short: "Local AI coding CLI (Ollama + Qwen)",
		Long:  "LocalCode — local coding agent CLI powered by Qwen models via Ollama.",
		RunE:  runChat,
	}

	root.PersistentFlags().StringVar(&flagMode, "mode", "", "Model mode: fast|smart|code|auto")
	root.PersistentFlags().StringVar(&flagModel, "model", "", "Override Ollama model name")
	root.PersistentFlags().IntVar(&flagContext, "context", 0, "Override context window (num_ctx)")
	root.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")
	root.PersistentFlags().StringVar(&flagURL, "provider-url", "", "Override Ollama base URL")
	root.PersistentFlags().BoolVar(&flagNoTools, "no-tools", false, "Disable repository tools")

	root.AddCommand(
		newAskCmd(),
		newChatCmd(),
		newExplainCmd(),
		newReviewCmd(),
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
	cfg    *config.Config
	prov   provider.Provider
	router *router.Router
	repo   *repo.Repo
	tools  *tools.Registry
}

func loadRuntime() (*runtimeEnv, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if flagURL != "" {
		cfg.Provider.BaseURL = flagURL
	}
	p := provider.NewOllama(cfg.Provider.BaseURL)
	r := router.New(cfg)

	env := &runtimeEnv{cfg: cfg, prov: p, router: r}
	if !flagNoTools {
		if rp, err := repo.Find(""); err == nil {
			env.repo = rp
			env.tools = tools.NewReadOnly(rp)
		}
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
		Verbose:  flagVerbose,
	}
}

func modeOrDefault(cfg *config.Config) string {
	if flagMode != "" {
		return flagMode
	}
	return cfg.Routing.Default
}

func newAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ask [prompt...]",
		Short: "One-shot question (uses repo tools when inside a git repo)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := strings.Join(args, " ")
			env, err := loadRuntime()
			if err != nil {
				return err
			}
			if err := env.prov.Ping(cmd.Context()); err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			_, err = env.agent().Run(ctx, agent.RunOptions{
				Prompt:  prompt,
				Mode:    modeOrDefault(env.cfg),
				Model:   flagModel,
				Context: flagContext,
			})
			return err
		},
	}
}

func newExplainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain [path]",
		Short: "Explain a file, directory, or the repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := loadRuntime()
			if err != nil {
				return err
			}
			if err := env.prov.Ping(cmd.Context()); err != nil {
				return err
			}
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			prompt := fmt.Sprintf(
				"Explain %q in this repository. Use tools to inspect it. Cover purpose, key parts, and how it fits the project. Be concise.",
				target,
			)
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			mode := modeOrDefault(env.cfg)
			if flagMode == "" {
				mode = "fast"
			}
			_, err = env.agent().Run(ctx, agent.RunOptions{
				Prompt:  prompt,
				Mode:    mode,
				Model:   flagModel,
				Context: flagContext,
			})
			return err
		},
	}
}

func newReviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "review",
		Short: "Review the current git diff",
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := loadRuntime()
			if err != nil {
				return err
			}
			if env.repo == nil {
				return fmt.Errorf("not inside a git repository")
			}
			if err := env.prov.Ping(cmd.Context()); err != nil {
				return err
			}
			prompt := "Review the current git working tree changes. Use git_status and git_diff. Report bugs, risks, and missing tests. Be concrete."
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			mode := modeOrDefault(env.cfg)
			if flagMode == "" {
				mode = "smart"
			}
			_, err = env.agent().Run(ctx, agent.RunOptions{
				Prompt:  prompt,
				Mode:    mode,
				Model:   flagModel,
				Context: flagContext,
			})
			return err
		},
	}
}

func newChatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Interactive repository-aware conversation",
		RunE:  runChat,
	}
}

func runChat(cmd *cobra.Command, _ []string) error {
	env, err := loadRuntime()
	if err != nil {
		return err
	}
	if err := env.prov.Ping(cmd.Context()); err != nil {
		return fmt.Errorf("%w\nIs Ollama running? Try: ollama serve", err)
	}

	repoLabel := "(no git repo)"
	if env.repo != nil {
		repoLabel = env.repo.Name()
	}
	ui.Infof("LocalCode chat  ·  repo:%s  ·  Ctrl+C or /exit to quit", repoLabel)
	ui.Infof("Commands: /mode /model /status /tools /clear /exit")

	mode := modeOrDefault(env.cfg)
	ag := env.agent()

	// Multi-turn: keep appending to a shared history by wrapping agent lightly.
	historyPromptPrefix := ""

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		fmt.Fprint(os.Stderr, "\n> ")
		if !in.Scan() {
			break
		}
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case lower == "/exit" || lower == "/quit":
			return nil
		case lower == "/status":
			printStatus(env, mode)
			continue
		case lower == "/tools":
			if env.tools == nil {
				ui.Infof("tools: disabled or no repo")
			} else {
				for _, d := range env.tools.Definitions() {
					ui.Infof("  %s — %s", d.Name, d.Description)
				}
			}
			continue
		case strings.HasPrefix(lower, "/mode "):
			mode = strings.TrimSpace(line[6:])
			ui.Infof("mode set to %s", mode)
			continue
		case strings.HasPrefix(lower, "/model "):
			flagModel = strings.TrimSpace(line[7:])
			ui.Infof("model override: %s", flagModel)
			continue
		case lower == "/clear":
			historyPromptPrefix = ""
			ui.Infof("history cleared")
			continue
		}

		prompt := line
		if historyPromptPrefix != "" {
			prompt = historyPromptPrefix + "\n\nUser: " + line
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
		answer, err := ag.Run(ctx, agent.RunOptions{
			Prompt:  prompt,
			Mode:    mode,
			Model:   flagModel,
			Context: flagContext,
		})
		stop()
		if err != nil {
			ui.Infof("error: %v", err)
			continue
		}
		// Keep a compact rolling summary for next turn (full message history is per-run for now).
		historyPromptPrefix = trimHistory(historyPromptPrefix + "\nUser: " + line + "\nAssistant: " + answer)
	}
	return in.Err()
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
			env, err := loadRuntime()
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
			env, err := loadRuntime()
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
	if env.repo != nil {
		fmt.Printf("repo:         %s\n", env.repo.Root)
		if env.tools != nil {
			fmt.Printf("tools:        %d enabled (read-only)\n", len(env.tools.Definitions()))
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
			fmt.Println("localcode 0.2.0-m2")
		},
	}
}
