package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/invopop/jsonschema"
	"os"
)

const systemPrompt = `
You are an autonomous agent working in a project repository.
Follow the goal given below:
`

type ListFilesInput struct {
	Directory string `json:"directory" jsonschema_description:"Path of the directory"`
}

var ListFileInputSchema = GenerateSchema[ListFilesInput]()

func GenerateSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T

	schema := reflector.Reflect(v)

	return anthropic.ToolInputSchemaParam{
		Properties: schema.Properties,
	}
}

func main() {
	// integrate with claude
	anthropicApiKey := os.Getenv("ANTHROPIC_API_KEY")
	client := anthropic.NewClient(
		option.WithAPIKey(anthropicApiKey), // defaults to os.LookupEnv("ANTHROPIC_API_KEY")
	)

	var goal = flag.String("goal", "", "What would you like the agent to do?")

	flag.Parse()
	userGoal := *goal

	inputMessages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(systemPrompt)),
		anthropic.NewUserMessage(anthropic.NewTextBlock("Goal: " + userGoal)),
	}

	// agent loop

	// tool definition
	toolParams := []anthropic.ToolParam{
		{
			Name:        "list_files",
			Description: anthropic.String("Returns a list of files in the current directory."),
			InputSchema: ListFileInputSchema,
		},
	}
	tools := make([]anthropic.ToolUnionParam, len(toolParams))
	for i, toolParam := range toolParams {
		tools[i] = anthropic.ToolUnionParam{OfTool: &toolParam}
	}

	for {
		// run inference.
		respMessage := runInference(client, inputMessages, tools)

		// print the response
		for _, block := range respMessage.Content {
			switch block := block.AsAny().(type) {
			case anthropic.TextBlock:
				fmt.Println(block)
			case anthropic.ToolUseBlock:
				inputJson, _ := json.MarshalIndent(block.Input, "", "  ")
				fmt.Println(block.Name + ": " + string(inputJson))
			}
		}
		// add the llm resp to conversation history
		inputMessages = append(inputMessages, respMessage.ToParam())

		// execute tool if present
		toolResult := []anthropic.ContentBlockParamUnion{}
		for _, block := range respMessage.Content {
			switch block := block.AsAny().(type) {
			case anthropic.ToolUseBlock:

				var response interface{}
				var toolErr error
				switch block.Name {
				case "list_files":
					var input struct {
						Directory string `json:"directory"`
					}

					err := json.Unmarshal(block.Input, &input)
					if err != nil {
						panic(err)
					}

					response, toolErr = listFile(input.Directory)
				}

				if toolErr != nil {
					toolResult = append(toolResult, anthropic.NewToolResultBlock(block.ID, toolErr.Error(), true))
				} else {
					b, err := json.Marshal(response)
					if err != nil {
						panic(err)
					}
					fmt.Println(string(b))
					toolResult = append(toolResult, anthropic.NewToolResultBlock(block.ID, string(b), false))
				}
			}
		}

		if len(toolResult) == 0 {
			break
		}
		inputMessages = append(inputMessages, anthropic.NewUserMessage(toolResult...))

		// if there tool calls. execute them. append tool result.

	}

	// tools call for filesystem

}

type ListFilesOutput struct {
	Files []string `json:"files"`
}

func listFile(directory string) (*ListFilesOutput, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("error reading directory: %w", err)
	}

	var files []string

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		files = append(files, name)
	}
	return &ListFilesOutput{
		Files: files,
	}, nil
}

func runInference(client anthropic.Client, input []anthropic.MessageParam, tools []anthropic.ToolUnionParam) *anthropic.Message {
	message, err := client.Messages.New(context.Background(), anthropic.MessageNewParams{
		MaxTokens: 1024,
		Messages:  input,
		Model:     anthropic.ModelClaudeSonnet4_5_20250929,
		Tools:     tools,
	})
	if err != nil {
		panic(err.Error())
	}
	return message
}
