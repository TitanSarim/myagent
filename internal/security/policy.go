package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Class int

const (
	ClassAuto Class = iota
	ClassApproval
	ClassBlocked
)

func (c Class) String() string {
	switch c {
	case ClassAuto:
		return "auto"
	case ClassApproval:
		return "approval"
	case ClassBlocked:
		return "blocked"
	default:
		return "unknown"
	}
}

// ClassifyCommand classifies argv (no shell interpolation).
func ClassifyCommand(argv []string) Class {
	if len(argv) == 0 {
		return ClassBlocked
	}
	bin := filepath.Base(argv[0])
	bin = strings.TrimSuffix(bin, ".exe")
	args := argv[1:]

	switch bin {
	case "sudo", "su", "doas", "mkfs", "reboot", "shutdown", "halt", "poweroff", "dd":
		return ClassBlocked
	}

	joined := strings.Join(argv, " ")
	lower := strings.ToLower(joined)
	if strings.Contains(lower, "rm -rf /") || strings.Contains(lower, "rm -fr /") {
		return ClassBlocked
	}
	if bin == "rm" {
		for _, a := range args {
			if a == "-rf" || a == "-fr" || a == "-r" || a == "-f" {
				return ClassBlocked
			}
		}
	}
	if bin == "git" && len(args) >= 1 {
		if args[0] == "push" {
			for _, a := range args {
				if a == "--force" || a == "-f" {
					return ClassBlocked
				}
			}
			return ClassApproval
		}
		if args[0] == "checkout" || args[0] == "reset" || args[0] == "clean" || args[0] == "rebase" {
			return ClassApproval
		}
		if args[0] == "status" || args[0] == "diff" || args[0] == "log" || args[0] == "rev-parse" || args[0] == "ls-files" || args[0] == "show" || args[0] == "branch" {
			return ClassAuto
		}
	}

	autoBins := map[string]bool{
		"pwd": true, "ls": true, "rg": true, "cat": true, "head": true, "tail": true,
		"echo": true, "true": true, "false": true, "which": true, "whoami": true,
		"go": true, "python": true, "python3": true, "pytest": true, "node": true,
		"npm": true, "npx": true, "yarn": true, "pnpm": true, "cargo": true,
		"make": true, "cmake": true, "rustc": true, "gcc": true, "clang": true,
	}
	approvalBins := map[string]bool{
		"pip": true, "pip3": true, "docker": true, "podman": true,
		"kubectl": true, "apt": true, "apt-get": true, "brew": true,
		"curl": true, "wget": true, "chmod": true, "chown": true, "mv": true, "cp": true,
	}

	if bin == "go" && len(args) > 0 {
		switch args[0] {
		case "test", "vet", "fmt", "build", "list", "env", "version", "mod":
			return ClassAuto
		case "get", "install":
			return ClassApproval
		}
	}
	if bin == "npm" && len(args) > 0 {
		switch args[0] {
		case "test", "run", "exec":
			return ClassAuto
		case "install", "i", "ci", "uninstall", "publish":
			return ClassApproval
		}
	}
	if bin == "pytest" || bin == "python" || bin == "python3" {
		return ClassAuto
	}
	if autoBins[bin] {
		return ClassAuto
	}
	if approvalBins[bin] {
		return ClassApproval
	}
	// Unknown binaries require approval
	return ClassApproval
}

func Describe(argv []string) string {
	return fmt.Sprintf("%v [%s]", argv, ClassifyCommand(argv))
}
