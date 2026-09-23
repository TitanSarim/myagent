package ui

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var Out io.Writer = os.Stdout
var ErrOut io.Writer = os.Stderr

func Infof(format string, args ...any) {
	fmt.Fprintf(ErrOut, format+"\n", args...)
}

func Header(mode, model, reason string) {
	parts := []string{"LocalCode"}
	if mode != "" {
		parts = append(parts, "mode:"+mode)
	}
	if model != "" {
		parts = append(parts, "model:"+model)
	}
	if reason != "" && reason != "explicit" {
		parts = append(parts, "("+reason+")")
	}
	Infof("%s", strings.Join(parts, "  ·  "))
}

func StreamWrite(s string) {
	fmt.Fprint(Out, s)
}

func Newline() {
	fmt.Fprintln(Out)
}

// Splash prints the interactive terminal welcome banner.
func Splash(appName, version, repo string, writable bool, mode string) {
	w := ErrOut
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  ╭──────────────────────────────────────────╮")
	fmt.Fprintf(w, "  │  %-40s │\n", appName)
	fmt.Fprintf(w, "  │  local coding agent  ·  %-16s │\n", "v"+version)
	fmt.Fprintln(w, "  ╰──────────────────────────────────────────╯")
	fmt.Fprintln(w)
	if repo != "" && repo != "(no git repo)" {
		fmt.Fprintf(w, "  repo     %s\n", repo)
	} else {
		fmt.Fprintln(w, "  repo     (none — cd into a git repo)")
	}
	fmt.Fprintf(w, "  mode     %s\n", mode)
	fmt.Fprintf(w, "  writes   %v\n", writable)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Type a request, or:")
	fmt.Fprintln(w, "    /mode fast|smart|code|auto")
	fmt.Fprintln(w, "    /write on|off   /tools   /status")
	fmt.Fprintln(w, "    /exit")
	fmt.Fprintln(w)
}

func Prompt() {
	fmt.Fprint(ErrOut, "› ")
}
