package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/agentengineering.dev/agent-framework/tool"
	"github.com/joho/godotenv"
	"log"
)

const systemPrompt = `
You are an autonomous agent working in a project repository.
Follow the goal given below:
`

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	var goal = flag.String("goal", "", "What would you like the agent to do?")

	flag.Parse()
	userGoal := *goal

	client, err := llm.NewClient("anthropic")
	if err != nil {
		log.Fatal(err)
	}

	inputMessages := []llm.Message{
		{
			Role: llm.RoleUser,
			Text: systemPrompt,
		},
		{
			Role: llm.RoleUser,
			Text: userGoal,
		},
	}

	// agent loop

	// tool definition

	allTools := []llm.ToolDefinition{
		tool.ListFilesToolDefinition,
		tool.ReadFileToolDefinition,
	}

	for {
		// run inference.
		respMessage, err := client.RunInference(inputMessages, allTools)
		if err != nil {
			log.Fatal(err)
		}

		// print the response
		for _, message := range respMessage {
			if message.Text != "" {
				fmt.Println(message.Text)
			} else if message.ToolUse != nil {
				inputJson, _ := json.MarshalIndent(message.ToolUse.Input, "", "  ")
				fmt.Println(message.ToolUse.Name + ": " + string(inputJson))
			}
		}
		// add the llm resp to conversation history
		inputMessages = append(inputMessages, respMessage...)

		// execute tool if present
		toolResult := []llm.ToolResult{}
		for _, block := range respMessage {
			if block.ToolUse != nil {

				toolResp, toolErr := tool.ExecuteTool(block.ToolUse.Name, block.ToolUse.Input)

				if toolErr != nil {
					toolResult = append(toolResult, llm.ToolResult{
						ID:      block.ToolUse.ID,
						IsError: true,
						Content: toolErr.Error(),
					})
				} else {
					toolResult = append(toolResult, llm.ToolResult{
						ID:      block.ToolUse.ID,
						IsError: false,
						Content: toolResp,
					})
				}
			}
		}

		if len(toolResult) == 0 {
			break
		}
		for _, tr := range toolResult {
			inputMessages = append(inputMessages, llm.Message{
				ToolResult: &tr,
			})
		}
	}
}
