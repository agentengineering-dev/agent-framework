package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"os"
)

type openAILLM struct {
	client openai.Client
}

func NewOpenAILLM() *openAILLM {
	openAIApiKey := os.Getenv("OPENAI_API_KEY")
	client := openai.NewClient(
		option.WithAPIKey(openAIApiKey),
	)
	return &openAILLM{
		client: client,
	}
}

func (o *openAILLM) RunInference(messages []Message, tools []ToolDefinition) ([]Message, error) {
	openAIMessages := transformToOpenAIMessages(messages)
	openAITools, err := transformToOpenAITools(tools)
	if err != nil {
		return nil, err
	}

	chatCompletion, err := o.client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Messages: openAIMessages,
		Model:    openai.ChatModelGPT5_2,
		Tools:    openAITools,
	})
	if err != nil {
		return nil, err
	}

	responseMessages := []Message{}

	if len(chatCompletion.Choices) > 0 {
		toolCalls := chatCompletion.Choices[0].Message.ToolCalls
		msg := chatCompletion.Choices[0].Message

		if msg.Content != "" {
			responseMessages = append(responseMessages, Message{
				Type: MessageTypeText,
				Text: msg.Content,
				Role: transformRole(string(msg.Role)),
			})
		}
		for _, call := range toolCalls {
			toolCall := call.AsFunction()
			responseMessages = append(responseMessages, Message{
				Type: MessageTypeToolUse,
				ToolUse: &ToolUse{
					ID:    toolCall.ID,
					Name:  toolCall.Function.Name,
					Input: json.RawMessage(toolCall.Function.Arguments),
				},
			})
		}
	}

	return responseMessages, nil
}

func transformToOpenAITools(tools []ToolDefinition) ([]openai.ChatCompletionToolUnionParam, error) {
	var openAITools []openai.ChatCompletionToolUnionParam
	for _, tool := range tools {

		params, err := GenerateOpenAISchema(tool.InputSchemaInstance)
		if err != nil {
			return nil, err
		}

		openAITools = append(openAITools, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        tool.Name,
					Strict:      openai.Bool(true),
					Description: openai.String(tool.Description),
					Parameters:  params,
				},
				Type: "function", // fixme
			},
		})
	}
	return openAITools, nil
}

func GenerateOpenAISchema(instance interface{}) (map[string]any, error) {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}

	schema := reflector.Reflect(instance)
	schemaMap, err := schema.MarshalJSON()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to marshal schema: %+v", instance))
	}
	result := make(map[string]any)
	err = json.Unmarshal(schemaMap, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func transformToOpenAIMessages(messages []Message) []openai.ChatCompletionMessageParamUnion {
	openAIMessages := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		switch msg.Type {
		case MessageTypeText:
			if msg.Role == RoleUser {
				openAIMessages[i] = openai.UserMessage(msg.Text)
			} else if msg.Role == RoleAssistant {
				openAIMessages[i] = openai.AssistantMessage(msg.Text)
			}
		case MessageTypeToolUse:
			openAIMessages[i] = openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{
						{
							OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
								ID: msg.ToolUse.ID,
								Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
									Arguments: string(msg.ToolUse.Input),
									Name:      msg.ToolUse.Name,
								},
							},
						},
					},
				},
			}
		case MessageTypeToolResult:
			resultContent := msg.ToolResult.Content
			if msg.ToolResult.IsError {
				resultContent = "Error: " + resultContent
			}
			openAIMessages[i] = openai.ToolMessage(resultContent, msg.ToolResult.ID)
		}
	}
	return openAIMessages

}
