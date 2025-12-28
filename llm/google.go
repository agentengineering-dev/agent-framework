package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/invopop/jsonschema"
	"google.golang.org/genai"
	"os"
)

type googleClient struct {
	client *genai.Client
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

func (g googleClient) RunInference(messages []Message, tools []ToolDefinition) ([]Message, error) {
	googleMessages := transformToGoogleMessages(messages)
	googleTools := transformToGoogleTools(tools)

	cfg := &genai.GenerateContentConfig{
		Tools: googleTools,
	}
	googleResponse, err := g.client.Models.GenerateContent(context.Background(), "gemini-3-pro-preview", googleMessages, cfg)
	if err != nil {
		return nil, err
	}
	var response []Message
	if len(googleResponse.Candidates) > 0 {
		response = transformFromGoogleMessage(googleResponse.Candidates[0].Content)
	}
	return response, nil
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
