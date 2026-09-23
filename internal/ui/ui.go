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
