package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ollama/ollama/api"
	"reflect"
	"strings"
	"time"
)

type ollamaLLM struct {
	client *api.Client
}

func NewOllamaLLM() (*ollamaLLM, error) {
	client, err := api.ClientFromEnvironment()
	if err != nil {
		return nil, err
	}
	return &ollamaLLM{client: client}, nil
}

func (o *ollamaLLM) GenerateStructuredResponse(messages []Message, resp interface{}) (*InferenceMetadata, error) {
	ollamaMessages := transformToOllamaMessages(messages)

	tools := []api.Tool{
		{
			Type: "function",
			Function: api.ToolFunction{
				Name:        "generate_structured_response",
				Description: "Given the goal generate a json response",
				Parameters:  GenerateOllamaSchema(resp),
			},
		},
	}

	var toolCall *api.ToolCall
	var responses []api.ChatResponse
	start := time.Now()
	err := o.client.Chat(context.Background(), &api.ChatRequest{
		Model:    "qwen3-coder",
		Messages: ollamaMessages,
		Tools:    tools,
	}, func(response api.ChatResponse) error {
		responses = append(responses, response)
		if len(response.Message.ToolCalls) > 0 {
			tc := response.Message.ToolCalls[0]
			toolCall = &tc
		}
		return nil
	})

	timeTaken := time.Since(start)

	if err != nil {
		return nil, err
	}

	if toolCall == nil {
		return nil, errors.New("tool call not invoked")
	}

	err = json.Unmarshal([]byte(toolCall.Function.Arguments.String()), resp)
	if err != nil {
		return nil, err
	}
	return ollamaMetadata(MODEL_OLLAMA_QWEN3_CODER, responses, timeTaken), nil

}

func (o *ollamaLLM) RunInference(messages []Message, tools []ToolDefinition) ([]Message, *InferenceMetadata, error) {
	ollamaMessages := transformToOllamaMessages(messages)
	ollamaTools := transformToOllamaTools(tools)

	var ollamaResponseMessages []api.ChatResponse
	start := time.Now()
	streamCfg := false
	err := o.client.Chat(context.Background(), &api.ChatRequest{
		Model:    "qwen3-coder",
		Messages: ollamaMessages,
		Tools:    ollamaTools,
		Stream:   &streamCfg,
	}, func(response api.ChatResponse) error {
		ollamaResponseMessages = append(ollamaResponseMessages, response)
		return nil
	})

	timeTaken := time.Since(start)
	metadata := ollamaMetadata(MODEL_OLLAMA_QWEN3_CODER, ollamaResponseMessages, timeTaken)
	if err != nil {
		return nil, metadata, err
	}

	respMessages := transformFromOllamaMessage(ollamaResponseMessages)

	return respMessages, metadata, nil

}

// ollamaMetadata sums the eval counts ollama reports. Nothing is billed for a
// local model, the token counts are still what the ui gauges run on.
func ollamaMetadata(model string, responses []api.ChatResponse, timeTaken time.Duration) *InferenceMetadata {
	md := &InferenceMetadata{
		Cost:          0,
		Time:          timeTaken,
		Provider:      PROVIDER_OLLAMA,
		Model:         model,
		ContextWindow: ContextWindowOf(PROVIDER_OLLAMA, model),
	}
	for _, r := range responses {
		md.InputTokens += int64(r.PromptEvalCount)
		md.OutputTokens += int64(r.EvalCount)
	}
	return md
}

func transformToOllamaTools(tools []ToolDefinition) []api.Tool {
	var out []api.Tool
	for _, t := range tools {
		out = append(out, api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  GenerateOllamaSchema(t.InputSchemaInstance),
			},
		})
	}
	return out
}

func transformFromOllamaMessage(messages []api.ChatResponse) []Message {
	var out []Message
	for _, m := range messages {
		if m.Message.Content != "" {
			out = append(out, Message{
				Type: MessageTypeText,
				Text: m.Message.Content,
			})
		}
		for _, t := range m.Message.ToolCalls {
			out = append(out, Message{
				Type: MessageTypeToolUse,
				ToolUse: &ToolUse{
					ID:    t.ID,
					Name:  t.Function.Name,
					Input: json.RawMessage(t.Function.Arguments.String()),
				},
			})
		}
	}
	return out

}

func transformToOllamaMessages(messages []Message) []api.Message {
	var out []api.Message
	for _, m := range messages {
		switch m.Type {
		case MessageTypeText:
			role := "user"
			if m.Role == RoleAssistant {
				role = "assistant"
			}
			out = append(out, api.Message{
				Role:    role,
				Content: m.Text,
			})
		case MessageTypeToolUse:
			var args map[string]any
			err := json.Unmarshal([]byte(m.ToolUse.Input), &args)
			if err != nil {
				fmt.Println("Error unmarshalling tool use", m.ToolUse.Name)
				return nil
			}
			out = append(out, api.Message{
				Role:       "assistant",
				ToolCallID: m.ToolUse.ID,
				ToolName:   m.ToolUse.Name,
				ToolCalls: []api.ToolCall{
					{
						ID: m.ToolUse.ID,
						Function: api.ToolCallFunction{
							Name:      m.ToolUse.Name,
							Arguments: args,
						},
					},
				},
			})
		case MessageTypeToolResult:
			out = append(out, api.Message{
				Role:       "user",
				ToolCallID: m.ToolResult.ID,
				ToolName:   m.ToolResult.ToolName,
				Content:    m.ToolResult.Content,
			})
		}
	}
	return out
}

func GenerateOllamaSchema(inst interface{}) api.ToolFunctionParameters {
	t := reflect.TypeOf(inst)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	props := map[string]api.ToolProperty{}
	var required []string

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		if !f.IsExported() {
			continue
		}

		name := jsonFieldName(f)
		if name == "-" {
			continue
		}

		prop := generateToolProperty(f.Type, f.Tag)

		props[name] = prop

		if isRequiredField(f) {
			required = append(required, name)
		}
	}

	return api.ToolFunctionParameters{
		Type:       "object",
		Properties: props,
		Required:   required,
	}
}

func generateToolProperty(t reflect.Type, tag reflect.StructTag) api.ToolProperty {
	if t.Kind() == reflect.Pointer {
		return generateToolProperty(t.Elem(), tag)
	}

	switch t.Kind() {

	case reflect.String:
		return api.ToolProperty{
			Type:        api.PropertyType{"string"},
			Description: tag.Get("description"),
			Enum:        enumValues(tag),
		}

	case reflect.Bool:
		return api.ToolProperty{
			Type: api.PropertyType{"boolean"},
		}

	case reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64:
		return api.ToolProperty{
			Type: api.PropertyType{"integer"},
		}

	case reflect.Float32, reflect.Float64:
		return api.ToolProperty{
			Type: api.PropertyType{"number"},
		}

	case reflect.Slice, reflect.Array:
		return api.ToolProperty{
			Type:  api.PropertyType{"array"},
			Items: generateToolProperty(t.Elem(), tag),
		}

	case reflect.Struct:
		props := map[string]api.ToolProperty{}
		var required []string

		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}

			name := jsonFieldName(f)
			if name == "-" {
				continue
			}

			props[name] = generateToolProperty(f.Type, f.Tag)

			if isRequiredField(f) {
				required = append(required, name)
			}
		}

		return api.ToolProperty{
			Type:       api.PropertyType{"object"},
			Properties: props,
		}

	default:
		return api.ToolProperty{
			Type: api.PropertyType{"string"},
		}
	}
}

func isRequiredField(f reflect.StructField) bool {
	// Pointer = optional
	if f.Type.Kind() == reflect.Pointer {
		return false
	}

	// json:",omitempty" = optional
	jsonTag := f.Tag.Get("json")
	return !strings.Contains(jsonTag, "omitempty")
}

func jsonFieldName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return strings.ToLower(f.Name)
	}
	return strings.Split(tag, ",")[0]
}

func enumValues(tag reflect.StructTag) []any {
	enumTag := tag.Get("enum")
	if enumTag == "" {
		return nil
	}

	parts := strings.Split(enumTag, ",")
	out := make([]any, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}
