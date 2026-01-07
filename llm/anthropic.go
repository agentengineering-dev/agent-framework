package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/invopop/jsonschema"
	"os"
	"strings"
	"time"
)

type anthropicLLM struct {
	client anthropic.Client
}

func (a *anthropicLLM) GenerateStructuredResponse(messages []Message, resp interface{}) (*Metadata, error) {
	anthropicMessages := transformToAnthropicMessages(messages)
	var anthropicTools = transformToAnthropicTools([]ToolDefinition{
		{
			Name:                "generate_structured_response",
			Description:         "Given the goal generate a json response",
			InputSchemaInstance: resp,
		},
	})
	now := time.Now()
	anthropicRespMessage, err := a.client.Messages.New(context.Background(), anthropic.MessageNewParams{
		MaxTokens:  1000 * 10,
		Messages:   anthropicMessages,
		Model:      anthropic.ModelClaudeSonnet4_5_20250929,
		Tools:      anthropicTools,
		ToolChoice: anthropic.ToolChoiceParamOfTool("generate_structured_response"),
	})
	if err != nil {
		return nil, err
	}
	timeTaken := time.Since(now)

	cost := computeAnthropicCost(string(anthropic.ModelClaudeSonnet4_5_20250929), anthropicRespMessage.Usage)

	for _, m := range anthropicRespMessage.Content {
		if m.Type == "tool_use" {
			toolUse := m.AsToolUse()
			err := json.Unmarshal(toolUse.Input, resp)

			if err != nil {
				return &Metadata{
					Cost: cost,
					Time: timeTaken,
				}, err
			}
		}
	}

	return &Metadata{
		Cost: cost,
		Time: timeTaken,
	}, nil
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

func (a *anthropicLLM) RunInference(messages []Message, tools []ToolDefinition) ([]Message, *Metadata, error) {
	anthropicMessages := transformToAnthropicMessages(messages)
	anthropicTools := transformToAnthropicTools(tools)
	now := time.Now()
	anthropicRespMessage, err := a.client.Messages.New(context.Background(), anthropic.MessageNewParams{
		MaxTokens: 1000 * 10,
		Messages:  anthropicMessages,
		Model:     anthropic.ModelClaudeSonnet4_5_20250929,
		Tools:     anthropicTools,
	})
	if err != nil {
		return nil, nil, err
	}
	timeTaken := time.Since(now)
	cost := computeAnthropicCost(string(anthropic.ModelClaudeSonnet4_5_20250929), anthropicRespMessage.Usage)

	if anthropicRespMessage.StopReason == "max_tokens" {
		return nil, &Metadata{Cost: cost, Time: timeTaken}, fmt.Errorf("max_tokens exceeded")
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

	return responseMessages, &Metadata{Cost: cost, Time: timeTaken}, nil

}

func computeAnthropicCost(model string, usage anthropic.Usage) float64 {
	cost := 0.0
	oneMil := 1000000.0
	inputPerM := 0.0
	outputPerM := 0.0
	cacheCreatePerM := 0.0
	cacheCreate1HPerM := 0.0
	cacheReadPerM := 0.0

	if strings.HasPrefix(model, "claude-opus-4.5") {
		inputPerM = 5
		outputPerM = 25
		cacheCreatePerM = 6.25
		cacheCreate1HPerM = 10
		cacheReadPerM = 0.5
	} else if strings.HasPrefix(model, "claude-sonnet-4.5") {
		if usage.InputTokens >= 2000000 {
			inputPerM = 6
			outputPerM = 22.5
			cacheCreatePerM = 3.75
			cacheCreate1HPerM = 12
			cacheReadPerM = 0.6
		} else {
			inputPerM = 3
			outputPerM = 15
			cacheCreatePerM = 7.5
			cacheCreate1HPerM = 12
			cacheReadPerM = 0.3
		}
	} else if strings.HasPrefix(model, "claude-haiku-4.5") {
		inputPerM = 1
		outputPerM = 5
		cacheCreatePerM = 1.25
		cacheCreate1HPerM = 2
		cacheReadPerM = 0.1
	}

	cost += float64(usage.InputTokens-usage.CacheReadInputTokens) * inputPerM / oneMil

	cost += float64(usage.CacheReadInputTokens) * cacheReadPerM / oneMil

	cost += float64(usage.CacheCreation.Ephemeral5mInputTokens) * cacheCreatePerM / oneMil

	cost += float64(usage.CacheCreation.Ephemeral1hInputTokens) * cacheCreate1HPerM / oneMil

	cost += float64(usage.OutputTokens) * outputPerM / oneMil

	return cost
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
