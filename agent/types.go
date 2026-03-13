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

type AgentSessionEvent struct {
	Type       AgentSessionEventType `json:"type"`
	Role       Role                  `json:"role"`
	Text       string                `json:"text"`
	ToolResult *ToolResult           `json:"tool_result"`
	ToolUse    *ToolUse              `json:"tool_use"`
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
	AgentSessionEventToolUse    AgentSessionEventType = "tool_use"
	AgentSessionEventToolResult AgentSessionEventType = "tool_result"
)

type Role string

const (
	RoleAgent     Role = "agent"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)
