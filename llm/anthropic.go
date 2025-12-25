package llm

import (
	"context"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/invopop/jsonschema"
	"os"
)

type anthropicLLM struct {
	client anthropic.Client
}

func NewAnthropicClient() *anthropicLLM {
	anthropicApiKey := os.Getenv("ANTHROPIC_API_KEY")
	client := anthropic.NewClient(
		option.WithAPIKey(anthropicApiKey), // defaults to os.LookupEnv("ANTHROPIC_API_KEY")
	)
	return &anthropicLLM{
		client: client,
	}
}

func (a *anthropicLLM) RunInference(messages []Message, tools []ToolDefinition) ([]Message, error) {
	anthropicMessages := transformToAnthropicMessages(messages)
	anthropicTools := transformToAnthropicTools(tools)
	anthropicRespMessage, err := a.client.Messages.New(context.Background(), anthropic.MessageNewParams{
		MaxTokens: 1024,
		Messages:  anthropicMessages,
		Model:     anthropic.ModelClaudeSonnet4_5_20250929,
		Tools:     anthropicTools,
	})
	if err != nil {
		return nil, err
	}
	responseMessages := []Message{}

	for _, m := range anthropicRespMessage.Content {
		if m.Type == "text" {
			responseMessages = append(responseMessages, Message{
				Role: transformRole(string(anthropicRespMessage.Role)),
				Text: m.Text,
				Type: MessageTypeText,
			})
		} else if m.Type == "tool_use" {
			toolUse := m.AsToolUse()
			responseMessages = append(responseMessages, Message{
				Type: MessageTypeToolUse,
				ToolUse: &ToolUse{
					ID:    toolUse.ID,
					Name:  toolUse.Name,
					Input: toolUse.Input,
				},
			})
		}
	}

	return responseMessages, nil

}

func transformRole(role string) Role {
	switch role {
	case "assistant":
		return RoleAssistant
	case "user":
		return RoleUser
	}
	return RoleUser
}

func transformToAnthropicTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
	toolParams := []anthropic.ToolParam{}
	for _, tool := range tools {
		toolParams = append(toolParams, anthropic.ToolParam{
			Name:        tool.Name,
			Description: anthropic.String(tool.Description),
			InputSchema: GenerateAnthropicSchema(tool.InputSchemaInstance),
		})
	}
	anthropicTools := make([]anthropic.ToolUnionParam, len(toolParams))
	for i, toolParam := range toolParams {
		anthropicTools[i] = anthropic.ToolUnionParam{OfTool: &toolParam}
	}
	return anthropicTools
}

func transformToAnthropicMessages(messages []Message) []anthropic.MessageParam {
	anthropicMessages := make([]anthropic.MessageParam, len(messages))
	for i, msg := range messages {
		switch msg.Type {
		case MessageTypeText:
			if msg.Role == RoleUser {
				anthropicMessages[i] = anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Text))
			} else if msg.Role == RoleAssistant {
				anthropicMessages[i] = anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Text))
			}
		case MessageTypeToolUse:
			anthropicMessages[i] = anthropic.NewAssistantMessage(anthropic.NewToolUseBlock(msg.ToolUse.ID, msg.ToolUse.Input, msg.ToolUse.Name))
		case MessageTypeToolResult:
			anthropicMessages[i] = anthropic.NewUserMessage(anthropic.NewToolResultBlock(msg.ToolResult.ID, msg.ToolResult.Content, msg.ToolResult.IsError))
		}

	}
	return anthropicMessages
}

func GenerateAnthropicSchema(inst interface{}) anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}

	schema := reflector.Reflect(inst)

	return anthropic.ToolInputSchemaParam{
		Properties: schema.Properties,
	}
}
