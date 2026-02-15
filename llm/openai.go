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
	"strings"
	"time"
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

func (o *openAILLM) RunInference(messages []Message, tools []ToolDefinition) ([]Message, *InferenceMetadata, error) {
	openAIMessages := transformToOpenAIMessages(messages)
	openAITools, err := transformToOpenAITools(tools)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	chatCompletion, err := o.client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Messages: openAIMessages,
		Model:    openai.ChatModelGPT5_2,
		Tools:    openAITools,
	})

	timeTaken := time.Since(now)

	if err != nil {
		return nil, nil, err
	}
	cost := computeOpenAICost(openai.ChatModelGPT5_2, chatCompletion.Usage)
	if chatCompletion.Choices[0].FinishReason == "length" {
		return nil, &InferenceMetadata{
			Cost: cost,
			Time: timeTaken,
		}, errors.New("max token exceeded")
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

	return responseMessages, &InferenceMetadata{
		Cost: cost,
		Time: timeTaken,
	}, nil
}

func computeOpenAICost(model string, usage openai.CompletionUsage) float64 {
	cost := 0.0

	oneMil := 1000000.0
	inputPerM := 0.0
	outputPerM := 0.0
	cacheReadPerM := 0.0

	if strings.HasPrefix(model, "gpt-5.2-pro") {
		inputPerM = 21
		outputPerM = 168
	} else if strings.HasPrefix(model, "gpt-5.2") {
		inputPerM = 1.75
		outputPerM = 14
		cacheReadPerM = 0.175
	} else if strings.HasPrefix(model, "gpt-5-mini") {
		inputPerM = 1.25
		outputPerM = 10
		cacheReadPerM = 0.125
	} else if strings.HasPrefix(model, "gpt-5-nano") {
		inputPerM = 0.05
		outputPerM = 0.4
		cacheReadPerM = 0.005
	}

	// add input token cost
	cost += float64(usage.PromptTokens-usage.PromptTokensDetails.CachedTokens) * inputPerM / oneMil

	// add cache read cost
	cost += float64(usage.PromptTokensDetails.CachedTokens) * cacheReadPerM / oneMil

	// add output token cost
	cost += float64(usage.CompletionTokens) * outputPerM / oneMil

	return cost
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

func (o openAILLM) GenerateStructuredResponse(messages []Message, resp interface{}) (*InferenceMetadata, error) {
	openAIMessages := transformToOpenAIMessages(messages)

	schema, err := GenerateOpenAISchema(resp)
	if err != nil {
		return nil, err
	}

	params := openai.ChatCompletionNewParams{
		Messages:            openAIMessages,
		Model:               openai.ChatModelGPT5_2,
		MaxCompletionTokens: openai.Int(1000 * 10),
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &shared.ResponseFormatJSONSchemaParam{
				JSONSchema: shared.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   "generate_structured_response",
					Strict: openai.Bool(true),
					Schema: schema,
				},
			},
		},
	}
	now := time.Now()
	chatCompletion, err := o.client.Chat.Completions.New(context.Background(), params)
	if err != nil {
		return nil, err
	}
	timeTaken := time.Since(now)

	cost := computeOpenAICost(openai.ChatModelGPT5_2, chatCompletion.Usage)

	if chatCompletion.Choices[0].FinishReason == "length" {
		return &InferenceMetadata{
			Cost: cost,
			Time: timeTaken,
		}, errors.New("max token exceeded")
	}

	if len(chatCompletion.Choices) > 0 {
		msg := chatCompletion.Choices[0].Message
		if msg.Content != "" {
			err := json.Unmarshal([]byte(msg.Content), resp)
			if err != nil {
				return &InferenceMetadata{
					Cost: cost,
					Time: timeTaken,
				}, err
			}
		}

	}

	return &InferenceMetadata{
		Cost: cost,
		Time: timeTaken,
	}, nil
}
