package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/invopop/jsonschema"
	"google.golang.org/genai"
	"os"
	"strings"
	"time"
)

type googleClient struct {
	client *genai.Client
}

func (g googleClient) GenerateStructuredResponse(messages []Message, resp interface{}) (*InferenceMetadata, error) {
	googleMessages := transformToGoogleMessages(messages)

	schema, err := GenerateGoogleSchema(resp)
	if err != nil {
		return nil, err
	}
	cfg := &genai.GenerateContentConfig{
		ResponseJsonSchema: schema,
		MaxOutputTokens:    1000 * 10,
	}
	now := time.Now()
	googleResponse, err := g.client.Models.GenerateContent(context.Background(), "gemini-3-pro-preview", googleMessages, cfg)
	if err != nil {
		return nil, err
	}
	timeTaken := time.Since(now)

	md := googleMetadata("gemini-3-pro-preview", googleResponse.UsageMetadata, timeTaken)

	if len(googleResponse.Candidates) > 0 {
		if googleResponse.Candidates[0].FinishReason == genai.FinishReasonMaxTokens {
			return md, fmt.Errorf("max_tokens exceeded")
		}

		if len(googleResponse.Candidates[0].Content.Parts) > 0 {
			var err = json.Unmarshal([]byte(googleResponse.Candidates[0].Content.Parts[0].Text), resp)
			if err != nil {
				return md, err
			}
		} else {
			return md, errors.New("no suitable result")
		}
	}
	return md, nil
}

func NewGoogleClient() (*googleClient, error) {
	key := os.Getenv("GOOGLE_API_KEY")
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  key,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}
	return &googleClient{client}, nil
}

func (g googleClient) RunInference(messages []Message, tools []ToolDefinition) ([]Message, *InferenceMetadata, error) {
	googleMessages := transformToGoogleMessages(messages)
	googleTools := transformToGoogleTools(tools)

	cfg := &genai.GenerateContentConfig{
		Tools:           googleTools,
		MaxOutputTokens: 1000 * 10,
	}
	now := time.Now()
	googleResponse, err := g.client.Models.GenerateContent(context.Background(), "gemini-3-pro-preview", googleMessages, cfg)
	if err != nil {
		return nil, nil, err
	}
	timeTaken := time.Since(now)
	md := googleMetadata("gemini-3-pro-preview", googleResponse.UsageMetadata, timeTaken)

	var response []Message
	if len(googleResponse.Candidates) > 0 {
		if googleResponse.Candidates[0].FinishReason == genai.FinishReasonMaxTokens {
			return nil, md, fmt.Errorf("max_tokens exceeded")
		}

		response = transformFromGoogleMessage(googleResponse.Candidates[0].Content)
	}
	return response, md, nil
}

// googleMetadata folds a gemini usage block into our metadata. The usage block
// is nil on some error paths, so it is treated as absent rather than empty.
func googleMetadata(model string, usage *genai.GenerateContentResponseUsageMetadata, timeTaken time.Duration) *InferenceMetadata {
	md := &InferenceMetadata{
		Time:          timeTaken,
		Provider:      PROVIDER_GOOGLE,
		Model:         model,
		ContextWindow: ContextWindowOf(PROVIDER_GOOGLE, model),
	}
	if usage == nil {
		return md
	}
	md.Cost = computeGoogleCost(model, usage)
	md.InputTokens = int64(usage.PromptTokenCount)
	md.OutputTokens = int64(usage.CandidatesTokenCount + usage.ThoughtsTokenCount)
	md.CachedTokens = int64(usage.CachedContentTokenCount)
	return md
}

func computeGoogleCost(model string, metadata *genai.GenerateContentResponseUsageMetadata) float64 {
	oneMil := 1000000.0
	inputPerM := 0.0
	outputPerM := 0.0
	cacheReadPerM := 0.0

	cost := 0.0

	if strings.HasPrefix(model, "gemini-3-pro") {
		if metadata.PromptTokenCount >= 200000 {
			inputPerM = 4
			outputPerM = 18
			cacheReadPerM = 0.4
		} else {
			inputPerM = 2
			outputPerM = 12
			cacheReadPerM = 0.2
		}
	} else if strings.HasPrefix(model, "gemini-3-flash") {
		inputPerM = 2
		outputPerM = 12
		cacheReadPerM = 0.2
	}
	cost += float64(metadata.PromptTokenCount-metadata.CachedContentTokenCount) * inputPerM / oneMil

	cost += float64(metadata.CachedContentTokenCount) * cacheReadPerM / oneMil
	cost += float64(metadata.CandidatesTokenCount) * outputPerM / oneMil
	return cost

}

func transformFromGoogleMessage(content *genai.Content) []Message {
	messages := make([]Message, 0)
	for _, part := range content.Parts {
		if part.Text != "" {
			messages = append(messages, Message{
				Type: MessageTypeText,
				Text: part.Text,
				Role: transformFromGoogleRol(content.Role),
			})
		}
		if part.FunctionCall != nil {
			toolArgs, _ := json.Marshal(part.FunctionCall.Args) // fixme
			messages = append(messages, Message{
				Type: MessageTypeToolUse,
				ToolUse: &ToolUse{
					ID:               part.FunctionCall.ID,
					Name:             part.FunctionCall.Name,
					Input:            toolArgs,
					ThoughtSignature: part.ThoughtSignature,
				},
			})
		}
	}
	return messages
}

func transformFromGoogleRol(role string) Role {
	switch role {
	case "user":
		return RoleUser
	case "model":
		return RoleAssistant
	}
	return RoleUser
}

func transformToGoogleMessages(messages []Message) []*genai.Content {
	googleMessages := []*genai.Content{}
	for _, message := range messages {
		switch message.Type {
		case MessageTypeText:
			googleMessages = append(googleMessages, &genai.Content{
				Role: transformToGoogleRole(message.Role),
				Parts: []*genai.Part{
					{
						Text: message.Text,
					},
				},
			})
		case MessageTypeToolUse:
			args := map[string]any{}
			_ = json.Unmarshal(message.ToolUse.Input, &args) // fixme
			googleMessages = append(googleMessages, &genai.Content{
				Role: transformToGoogleRole(RoleAssistant),
				Parts: []*genai.Part{
					{
						FunctionCall: &genai.FunctionCall{
							Name: message.ToolUse.Name,
							ID:   message.ToolUse.ID,
							Args: args,
						},
						ThoughtSignature: message.ToolUse.ThoughtSignature,
					},
				},
			})
		case MessageTypeToolResult:
			resp := map[string]any{}
			if message.ToolResult.IsError {
				resp["error"] = message.ToolResult.Content
			} else {
				resp["output"] = message.ToolResult.Content
			}
			googleMessages = append(googleMessages, &genai.Content{
				Role: transformToGoogleRole(RoleUser),
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name:     message.ToolResult.ToolName,
							ID:       message.ToolResult.ID,
							Response: resp,
						},
					},
				},
			})
		}
	}
	return googleMessages
}

func transformToGoogleRole(role Role) string {
	switch role {
	case RoleUser:
		return "user"
	case RoleAssistant:
		return "model"
	}
	return "user"
}

func transformToGoogleTools(tools []ToolDefinition) []*genai.Tool {
	googleTools := []*genai.Tool{}
	for _, tool := range tools {
		schema, err := GenerateGoogleSchema(tool.InputSchemaInstance)
		if err != nil {
			fmt.Println("failed to register tool", tool.Name, err.Error())
			continue
		}
		googleTools = append(googleTools, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:                 tool.Name,
					Description:          tool.Description,
					ParametersJsonSchema: schema,
				},
			},
		})
	}
	return googleTools
}

func GenerateGoogleSchema(inst interface{}) (any, error) {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	schema := reflector.Reflect(inst)
	schemaMap, err := schema.MarshalJSON()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("failed to marshal schema: %+v", inst))
	}
	result := make(map[string]any)
	err = json.Unmarshal(schemaMap, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
