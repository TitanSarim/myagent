package provider

import (
	"context"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall // assistant
	ToolName   string     // tool role
	ToolCallID string     // tool role
}

type ToolSpec struct {
	Raw map[string]any // already-shaped Ollama/OpenAI tool object
}

type ChatRequest struct {
	Model     string
	Messages  []Message
	Tools     []map[string]any
	Context   int
	KeepAlive string
	Stream    bool
}

type ModelInfo struct {
	Name   string
	Size   int64
	Family string
}

type EventKind string

const (
	EventToken     EventKind = "token"
	EventToolCalls EventKind = "tool_calls"
	EventDone      EventKind = "done"
	EventError     EventKind = "error"
)

type Event struct {
	Kind      EventKind
	Content   string
	ToolCalls []ToolCall
	Err       error
}

type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (<-chan Event, error)
	ListModels(ctx context.Context) ([]ModelInfo, error)
	Ping(ctx context.Context) error
}
