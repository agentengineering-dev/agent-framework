package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	// what the studio was started with, the model catalogue only knows the
	// default. UNSLOTH_CONTEXT_WINDOW overrides it.
	contextWindow int64
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

	apiKey := os.Getenv("UNSLOTH_API_KEY")

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	// the studio knows what it actually loaded, which is almost never the
	// model's native window. the catalogue is only the fallback for when it
	// cannot be reached.
	contextWindow := ContextWindowOf(PROVIDER_UNSLOTH, model)
	if served, err := fetchUnslothContextWindow(baseURL, apiKey, model); err != nil {
		fmt.Println("unsloth: falling back to the catalogued context window: " + err.Error())
	} else {
		contextWindow = served
	}

	// an explicit override still wins, the studio can be wrong about a model
	// it is proxying rather than serving itself.
	if override := os.Getenv("UNSLOTH_CONTEXT_WINDOW"); override != "" {
		parsed, err := strconv.ParseInt(strings.TrimSpace(override), 10, 64)
		if err != nil {
			return nil, errors.New("UNSLOTH_CONTEXT_WINDOW must be an integer, got: " + override)
		}
		contextWindow = parsed
	}

	return &unslothLLM{
		client:        client,
		model:         model,
		contextWindow: contextWindow,
	}, nil
}

// unslothModel is the part of an unsloth studio /v1/models entry we care
// about. The studio adds the context fields on top of the openai shape:
// context_length is what it is serving right now, native_context_length is
// what the model was trained with.
type unslothModel struct {
	ID                  string `json:"id"`
	ContextLength       int64  `json:"context_length"`
	MaxContextLength    int64  `json:"max_context_length"`
	NativeContextLength int64  `json:"native_context_length"`
	Loaded              bool   `json:"loaded"`
}

// fetchUnslothContextWindow asks the studio how large a context it is actually
// serving the model with. That served size, not the model's native window, is
// what the session has to fit inside, so it is what the ui gauges against.
func fetchUnslothContextWindow(baseURL string, apiKey string, model string) (int64, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"models", nil)
	if err != nil {
		return 0, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("studio returned %s for %smodels", resp.Status, baseURL)
	}

	var payload struct {
		Data []unslothModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, err
	}

	for _, m := range payload.Data {
		if m.ID != model {
			continue
		}
		// context_length is the loaded size, max_context_length is what the
		// studio would allow, either beats guessing.
		if m.ContextLength > 0 {
			return m.ContextLength, nil
		}
		if m.MaxContextLength > 0 {
			return m.MaxContextLength, nil
		}
		return 0, fmt.Errorf("studio reported no context length for %s", model)
	}

	return 0, fmt.Errorf("%s is not among the models the studio has loaded", model)
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
	metadata := o.metadata(chatCompletion.Usage, timeTaken)

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

	metadata := o.metadata(chatCompletion.Usage, timeTaken)

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

// metadata reports the studio's token counts, they are what the ui turns into
// the context gauge and the observed tokens per second.
func (o *unslothLLM) metadata(usage openai.CompletionUsage, timeTaken time.Duration) *InferenceMetadata {
	md := openAICompatMetadata(PROVIDER_UNSLOTH, o.model, usage, timeTaken, 0)
	md.ContextWindow = o.contextWindow
	return md
}
