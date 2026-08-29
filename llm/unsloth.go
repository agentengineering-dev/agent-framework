package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// unsloth studio exposes an OpenAI compatible API, so we reuse the openai
// client and the openai message/tool transformers.
type unslothLLM struct {
	client openai.Client
	model  string
}

func NewUnslothLLM() (*unslothLLM, error) {
	host := os.Getenv("UNSLOTH_API_HOST")
	if host == "" {
		host = DEFAULT_UNSLOTH_API_HOST
	}

	baseURL, err := unslothBaseURL(host)
	if err != nil {
		return nil, err
	}

	model := os.Getenv("UNSLOTH_MODEL")
	if model == "" {
		model = MODEL_UNSLOTH_QWEN3_27B
	}

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(os.Getenv("UNSLOTH_API_KEY")),
	)

	return &unslothLLM{
		client: client,
		model:  model,
	}, nil
}

// unslothBaseURL turns UNSLOTH_API_HOST (e.g. http://127.0.0.1:8888) into the
// base url the openai client expects (e.g. http://127.0.0.1:8888/v1/). The
// host may already carry the /v1 suffix, in which case it is left alone.
func unslothBaseURL(host string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(host))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", errors.New("UNSLOTH_API_HOST must be an absolute url, got: " + host)
	}

	path := strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(path, "/v1") {
		path += "/v1"
	}
	u.Path = path + "/"

	return u.String(), nil
}

func (o *unslothLLM) RunInference(messages []Message, tools []ToolDefinition) ([]Message, *InferenceMetadata, error) {
	unslothMessages := transformToOpenAIMessages(messages)
	unslothTools, err := transformToOpenAITools(tools)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	chatCompletion, err := o.client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Messages: unslothMessages,
		Model:    o.model,
		Tools:    unslothTools,
	})
	timeTaken := time.Since(now)

	if err != nil {
		return nil, nil, err
	}

	// self hosted inference, nothing to bill.
	metadata := &InferenceMetadata{
		Cost: 0,
		Time: timeTaken,
	}

	if len(chatCompletion.Choices) == 0 {
		return nil, metadata, errors.New("no choices returned")
	}

	if chatCompletion.Choices[0].FinishReason == "length" {
		return nil, metadata, errors.New("max token exceeded")
	}

	responseMessages := []Message{}
	msg := chatCompletion.Choices[0].Message

	if msg.Content != "" {
		responseMessages = append(responseMessages, Message{
			Type: MessageTypeText,
			Text: msg.Content,
			Role: transformRole(string(msg.Role)),
		})
	}
	for _, call := range msg.ToolCalls {
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

	return responseMessages, metadata, nil
}

func (o *unslothLLM) GenerateStructuredResponse(messages []Message, resp interface{}) (*InferenceMetadata, error) {
	unslothMessages := transformToOpenAIMessages(messages)

	schema, err := GenerateOpenAISchema(resp)
	if err != nil {
		return nil, err
	}

	params := openai.ChatCompletionNewParams{
		Messages:            unslothMessages,
		Model:               o.model,
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

	metadata := &InferenceMetadata{
		Cost: 0,
		Time: timeTaken,
	}

	if len(chatCompletion.Choices) == 0 {
		return metadata, errors.New("no choices returned")
	}

	if chatCompletion.Choices[0].FinishReason == "length" {
		return metadata, errors.New("max token exceeded")
	}

	content := chatCompletion.Choices[0].Message.Content
	if content == "" {
		return metadata, errors.New("empty structured response")
	}

	if err := json.Unmarshal([]byte(content), resp); err != nil {
		return metadata, err
	}

	return metadata, nil
}
