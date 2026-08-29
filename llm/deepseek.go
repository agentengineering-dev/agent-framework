package llm

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"os"
	"strings"
	"time"
)

type deepseekLLM struct {
	client openai.Client
}

func NewDeepSeekLLM() *deepseekLLM {
	deepseekApiKey := os.Getenv("DEEPSEEK_API_KEY")
	deepseekHost := os.Getenv("DEEPSEEK_HOST")
	client := openai.NewClient(
		option.WithBaseURL(deepseekHost),
		option.WithAPIKey(deepseekApiKey),
	)
	return &deepseekLLM{
		client: client,
	}
}

func (o *deepseekLLM) RunInference(messages []Message, tools []ToolDefinition) ([]Message, *InferenceMetadata, error) {
	openAIMessages := transformToOpenAIMessages(messages)
	openAITools, err := transformToOpenAITools(tools)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	chatCompletion, err := o.client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Messages: openAIMessages,
		Model:    MODEL_DEEPSEEK_CHAT,
		Tools:    openAITools,
	})

	timeTaken := time.Since(now)

	if err != nil {
		return nil, nil, err
	}
	md := openAICompatMetadata(PROVIDER_DEEPSEEK, MODEL_DEEPSEEK_CHAT, chatCompletion.Usage, timeTaken, computeDeepseekCost(MODEL_DEEPSEEK_CHAT, chatCompletion.Usage))
	if chatCompletion.Choices[0].FinishReason == "length" {
		return nil, md, errors.New("max token exceeded")
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

	return responseMessages, md, nil
}

func (o deepseekLLM) GenerateStructuredResponse(messages []Message, resp interface{}) (*InferenceMetadata, error) {
	openAIMessages := transformToOpenAIMessages(messages)

	deepseekTools, err := transformToOpenAITools([]ToolDefinition{
		{
			Name:                "generate_structured_response",
			Description:         "Given the goal generate a json response",
			InputSchemaInstance: resp,
		},
	})
	if err != nil {
		return nil, err
	}
	now := time.Now()

	params := openai.ChatCompletionNewParams{
		Messages:            openAIMessages,
		Model:               MODEL_DEEPSEEK_CHAT,
		MaxCompletionTokens: openai.Int(1000 * 10),
		Tools:               deepseekTools,
		ToolChoice: openai.ToolChoiceOptionFunctionToolChoice(openai.ChatCompletionNamedToolChoiceFunctionParam{
			Name: "generate_structured_response",
		}),
	}
	chatCompletion, err := o.client.Chat.Completions.New(context.Background(), params)
	if err != nil {
		return nil, err
	}
	timeTaken := time.Since(now)

	md := openAICompatMetadata(PROVIDER_DEEPSEEK, MODEL_DEEPSEEK_CHAT, chatCompletion.Usage, timeTaken, computeDeepseekCost(MODEL_DEEPSEEK_CHAT, chatCompletion.Usage))

	if chatCompletion.Choices[0].FinishReason == "length" {
		return md, errors.New("max token exceeded")
	}

	if len(chatCompletion.Choices) > 0 {
		toolCalls := chatCompletion.Choices[0].Message.ToolCalls
		for _, call := range toolCalls {
			toolCall := call.AsFunction()
			err := json.Unmarshal([]byte(toolCall.Function.Arguments), resp)
			if err != nil {
				return md, err
			}
		}

	}

	return md, nil
}

func computeDeepseekCost(model string, usage openai.CompletionUsage) float64 {
	cost := 0.0

	oneMil := 1000000.0
	inputPerM := 0.0
	outputPerM := 0.0
	cacheReadPerM := 0.0

	if strings.HasPrefix(model, MODEL_DEEPSEEK_CHAT) {
		inputPerM = 0.28
		cacheReadPerM = 0.028
		outputPerM = 0.42
	} else if strings.HasPrefix(model, MODEL_DEEPSEEK_REASONER) {
		inputPerM = 0.28
		cacheReadPerM = 0.028
		outputPerM = 0.42
	}

	// add input token cost
	cost += float64(usage.PromptTokens-usage.PromptTokensDetails.CachedTokens) * inputPerM / oneMil

	// add cache read cost
	cost += float64(usage.PromptTokensDetails.CachedTokens) * cacheReadPerM / oneMil

	// add output token cost
	cost += float64(usage.CompletionTokens) * outputPerM / oneMil

	return cost
}
