package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

type Call struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type Result struct {
	ID      string
	Name    string
	Content string
	IsError bool
}

type Definition struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema object
}

type Tool interface {
	Def() Definition
	Run(ctx context.Context, args map[string]any) (string, error)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry(ts ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(ts))}
	for _, t := range ts {
		r.tools[t.Def().Name] = t
	}
	return r
}

func (r *Registry) Definitions() []Definition {
	out := make([]Definition, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Def())
	}
	return out
}

func (r *Registry) Run(ctx context.Context, call Call) Result {
	t, ok := r.tools[call.Name]
	if !ok {
		return Result{
			ID:      call.ID,
			Name:    call.Name,
			Content: fmt.Sprintf("unknown tool: %s", call.Name),
			IsError: true,
		}
	}
	content, err := t.Run(ctx, call.Arguments)
	if err != nil {
		return Result{
			ID:      call.ID,
			Name:    call.Name,
			Content: err.Error(),
			IsError: true,
		}
	}
	return Result{ID: call.ID, Name: call.Name, Content: content}
}

// OllamaTools converts definitions to Ollama/OpenAI tool schema.
func (r *Registry) OllamaTools() []map[string]any {
	defs := r.Definitions()
	out := make([]map[string]any, 0, len(defs))
	for _, d := range defs {
		params := d.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        d.Name,
				"description": d.Description,
				"parameters":   params,
			},
		})
	}
	return out
}

func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func argInt(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		i, err := t.Int64()
		if err == nil {
			return int(i)
		}
	case string:
		var i int
		if _, err := fmt.Sscanf(t, "%d", &i); err == nil {
			return i
		}
	}
	return def
}

func argBool(args map[string]any, key string, def bool) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	}
	return def
}
