package llm

import (
	"encoding/json"
	"time"
)

type LLM interface {
	RunInference(messages []Message, tools []ToolDefinition) ([]Message, *InferenceMetadata, error)
	GenerateStructuredResponse(messages []Message, resp interface{}) (*InferenceMetadata, error)
}

type InferenceMetadata struct {
	Cost float64
	Time time.Duration

	// token accounting as reported by the provider. the ui reports these as
	// the context in use and the observed throughput, so every provider fills
	// them in even when it has nothing to bill.
	Provider      string
	Model         string
	InputTokens   int64
	OutputTokens  int64
	CachedTokens  int64
	ContextWindow int64
}

// ContextTokens is the size of the conversation after this inference, the
// prompt we sent plus what the model wrote back into it.
func (m *InferenceMetadata) ContextTokens() int64 {
	if m == nil {
		return 0
	}
	return m.InputTokens + m.OutputTokens
}

// TokensPerSecond is the output throughput we actually observed, wall clock
// around the http call included. 0 when we cannot tell.
func (m *InferenceMetadata) TokensPerSecond() float64 {
	if m == nil || m.Time <= 0 || m.OutputTokens <= 0 {
		return 0
	}
	return float64(m.OutputTokens) / m.Time.Seconds()
}

type MessageType string

const (
	MessageTypeText       MessageType = "text"
	MessageTypeToolUse    MessageType = "tool_use"
	MessageTypeToolResult MessageType = "tool_result"
)

type Message struct {
	Type       MessageType
	Role       Role
	Text       string
	ToolResult *ToolResult
	ToolUse    *ToolUse
}

type Role string

const (
	RoleAgent     Role = "agent"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type ToolUse struct {
	ID    string
	Name  string
	Input json.RawMessage
	// required by google
	ThoughtSignature []byte
}

type ToolResult struct {
	ID       string
	ToolName string
	Content  string
	IsError  bool
}

type ToolDefinition struct {
	Name                string
	Description         string
	InputSchemaInstance interface{}
	Func                func(json.RawMessage) (string, error)
}
