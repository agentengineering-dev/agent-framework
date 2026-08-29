package agent

import "encoding/json"

type CreateAgentSessionParams struct {
	Goal     string `json:"goal"`
	Provider string `json:"provider"`
}

type SessionMetadata struct {
	SessionName string `json:"session_name" jsonschema_description:"The name of the session"`
	BranchName  string `json:"branch_name" jsonschema_description:"The name of the branch"`
}

type SessionTitle struct {
	Title string `json:"title" jsonschema_description:"A short title summarizing the session"`
}

type AgentSessionEvent struct {
	Type       AgentSessionEventType `json:"type"`
	Role       Role                  `json:"role"`
	Text       string                `json:"text"`
	State      AgentState            `json:"state,omitempty"`
	ToolResult *ToolResult           `json:"tool_result"`
	ToolUse    *ToolUse              `json:"tool_use"`
	Usage      *Usage                `json:"usage,omitempty"`
}

// Usage is what the framework observed for the session so far. Context and
// throughput are measured, not estimated: they come from the token counts the
// provider reported and the wall clock the framework timed around the call.
type Usage struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`

	// the conversation the next inference will carry, and how much of the
	// model's window that is. ContextWindow is 0 for models we have not
	// catalogued, in which case ContextPercent is meaningless.
	ContextTokens  int64   `json:"context_tokens"`
	ContextWindow  int64   `json:"context_window"`
	ContextPercent float64 `json:"context_percent"`

	// output throughput, the last inference and the session average.
	TokensPerSecond    float64 `json:"tokens_per_second"`
	AvgTokensPerSecond float64 `json:"avg_tokens_per_second"`

	// session totals.
	Inferences    int     `json:"inferences"`
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	Cost          float64 `json:"cost"`
	InferenceTime float64 `json:"inference_time"`
	ElapsedTime   float64 `json:"elapsed_time"`

	// the inference that produced this snapshot.
	LastInputTokens   int64   `json:"last_input_tokens"`
	LastOutputTokens  int64   `json:"last_output_tokens"`
	LastCachedTokens  int64   `json:"last_cached_tokens"`
	LastInferenceTime float64 `json:"last_inference_time"`
}

type ToolUse struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type ToolResult struct {
	ID       string `json:"id"`
	ToolName string `json:"tool_name"`
	Content  string `json:"content"`
	IsError  bool   `json:"is_error"`
}

type AgentSessionEventType string

const (
	AgentSessionEventText       AgentSessionEventType = "text"
	AgentSessionEventTitle      AgentSessionEventType = "title"
	AgentSessionEventToolUse    AgentSessionEventType = "tool_use"
	AgentSessionEventToolResult AgentSessionEventType = "tool_result"
	AgentSessionEventUsage      AgentSessionEventType = "usage"
	AgentSessionEventStatus     AgentSessionEventType = "status"
)

// AgentState is what the agent is busy with, the ui turns it into a live
// indicator so a long inference does not look like a stalled session.
type AgentState string

const (
	AgentStateInferring AgentState = "inferring"
	AgentStateTool      AgentState = "tool"
	AgentStateIdle      AgentState = "idle"
	AgentStateDone      AgentState = "done"
	AgentStateError     AgentState = "error"
)

type Role string

const (
	RoleAgent     Role = "agent"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)
