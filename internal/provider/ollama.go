package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Ollama struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewOllama(baseURL string) *Ollama {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Ollama{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 0,
		},
	}
}

type ollamaChatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content,omitempty"`
	ToolCalls []struct {
		ID       string `json:"id,omitempty"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
		Type string `json:"type,omitempty"`
	} `json:"tool_calls,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type ollamaChatRequest struct {
	Model     string              `json:"model"`
	Messages  []ollamaChatMessage `json:"messages"`
	Stream    bool                `json:"stream"`
	KeepAlive string              `json:"keep_alive,omitempty"`
	Options   map[string]any      `json:"options,omitempty"`
	Tools     []map[string]any    `json:"tools,omitempty"`
}

type ollamaChatResponse struct {
	Message ollamaChatMessage `json:"message"`
	Done    bool              `json:"done"`
	Error   string            `json:"error,omitempty"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		Details struct {
			Family string `json:"family"`
		} `json:"details"`
	} `json:"models"`
}

func (o *Ollama) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable at %s: %w", o.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned %s", resp.Status)
	}
	return nil
}

func (o *Ollama) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list models: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	out := make([]ModelInfo, 0, len(tags.Models))
	for _, m := range tags.Models {
		out = append(out, ModelInfo{
			Name:   m.Name,
			Size:   m.Size,
			Family: m.Details.Family,
		})
	}
	return out, nil
}

func toOllamaMessages(msgs []Message) []ollamaChatMessage {
	out := make([]ollamaChatMessage, 0, len(msgs))
	for _, m := range msgs {
		om := ollamaChatMessage{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolName:   m.ToolName,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			args, _ := json.Marshal(tc.Arguments)
			om.ToolCalls = append(om.ToolCalls, struct {
				ID       string `json:"id,omitempty"`
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
				Type string `json:"type,omitempty"`
			}{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}{
					Name:      tc.Name,
					Arguments: args,
				},
			})
		}
		out = append(out, om)
	}
	return out
}

func parseToolCalls(msg ollamaChatMessage) []ToolCall {
	if len(msg.ToolCalls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(msg.ToolCalls))
	for i, tc := range msg.ToolCalls {
		args := map[string]any{}
		raw := bytes.TrimSpace(tc.Function.Arguments)
		if len(raw) > 0 {
			if raw[0] == '"' {
				// Sometimes arguments arrive as a JSON-encoded string.
				var s string
				if err := json.Unmarshal(raw, &s); err == nil {
					raw = []byte(s)
				}
			}
			_ = json.Unmarshal(raw, &args)
		}
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		out = append(out, ToolCall{
			ID:        id,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}
	return out
}

func (o *Ollama) Chat(ctx context.Context, req ChatRequest) (<-chan Event, error) {
	stream := req.Stream
	// Tool rounds are more reliable non-streaming.
	if len(req.Tools) > 0 {
		stream = false
	}

	body := ollamaChatRequest{
		Model:     req.Model,
		Messages:  toOllamaMessages(req.Messages),
		Stream:    stream,
		KeepAlive: req.KeepAlive,
		Tools:     req.Tools,
	}
	if req.Context > 0 {
		body.Options = map[string]any{"num_ctx": req.Context}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/api/chat", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := o.HTTPClient
	if client.Timeout != 0 {
		client = &http.Client{Timeout: 0}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chat: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("chat: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}

	ch := make(chan Event, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		if !stream {
			var chunk ollamaChatResponse
			if err := json.NewDecoder(resp.Body).Decode(&chunk); err != nil {
				ch <- Event{Kind: EventError, Err: err}
				return
			}
			if chunk.Error != "" {
				ch <- Event{Kind: EventError, Err: fmt.Errorf("%s", chunk.Error)}
				return
			}
			calls := parseToolCalls(chunk.Message)
			if len(calls) > 0 {
				ch <- Event{Kind: EventToolCalls, Content: chunk.Message.Content, ToolCalls: calls}
			} else if chunk.Message.Content != "" {
				ch <- Event{Kind: EventToken, Content: chunk.Message.Content}
			}
			ch <- Event{Kind: EventDone}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		var pendingCalls []ToolCall
		var content strings.Builder

		for scanner.Scan() {
			line := scanner.Bytes()
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var chunk ollamaChatResponse
			if err := json.Unmarshal(line, &chunk); err != nil {
				ch <- Event{Kind: EventError, Err: fmt.Errorf("decode stream: %w", err)}
				return
			}
			if chunk.Error != "" {
				ch <- Event{Kind: EventError, Err: fmt.Errorf("%s", chunk.Error)}
				return
			}
			if c := chunk.Message.Content; c != "" {
				content.WriteString(c)
				select {
				case <-ctx.Done():
					ch <- Event{Kind: EventError, Err: ctx.Err()}
					return
				case ch <- Event{Kind: EventToken, Content: c}:
				}
			}
			if calls := parseToolCalls(chunk.Message); len(calls) > 0 {
				pendingCalls = calls
			}
			if chunk.Done {
				if len(pendingCalls) > 0 {
					ch <- Event{Kind: EventToolCalls, Content: content.String(), ToolCalls: pendingCalls}
				}
				ch <- Event{Kind: EventDone}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- Event{Kind: EventError, Err: err}
			return
		}
		ch <- Event{Kind: EventDone}
	}()

	return ch, nil
}

func WaitHealthy(ctx context.Context, p Provider, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		last = p.Ping(c)
		cancel()
		if last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return last
}
