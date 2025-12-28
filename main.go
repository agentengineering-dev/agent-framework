package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/agentengineering.dev/agent-framework/git_helpers"
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

	var goal = flag.String("goal", "", "What would you like the agent to do?")
	var provider = flag.String("provider", "", "openai|anthropic|google")
	var envFile = flag.String("env-file", "", "The env file path")

	flag.Parse()
	userGoal := *goal
	userProvider := *provider
	envFileStr := *envFile

	err := godotenv.Load(envFileStr)
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	client, err := llm.NewClient(userProvider)
	if err != nil {
		log.Fatal(err)
	}

	inputMessages := []llm.Message{
		{
			Role: llm.RoleUser,
			Text: systemPrompt,
			Type: llm.MessageTypeText,
		},
		{
			Role: llm.RoleUser,
			Text: userGoal,
			Type: llm.MessageTypeText,
		},
	}

	// agent loop

	// tool definition

	allTools := []llm.ToolDefinition{}
	for _, t := range tool.ToolMap {
		allTools = append(allTools, t)
	}

	// git init.

	git_helpers.Init()

	// make llm inference for name?

	branch := "feature-123"
	// create a branch
	err = git_helpers.CreateBranch(branch)
	if err != nil {
		fmt.Println("failed to create branch: ", err.Error())
		return
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
				fmt.Println("Assistant: " + message.Text)
			} else if message.ToolUse != nil {
				inputJson, _ := json.MarshalIndent(message.ToolUse.Input, "", "  ")
				fmt.Println(fmt.Sprintf("Assistant: ToolUse: ID: %s, Name: %s, Input: %s", message.ToolUse.ID, message.ToolUse.Name, string(inputJson)))
			}
		}
		// add the llm resp to conversation history

		// execute tool if present
		hasToolUse := false
		for _, block := range respMessage {
			switch block.Type {
			case llm.MessageTypeText:
				inputMessages = append(inputMessages, block)
			case llm.MessageTypeToolUse:
				hasToolUse = true
				inputMessages = append(inputMessages, block)

				toolResp, toolErr := tool.ExecuteTool(block.ToolUse.Name, block.ToolUse.Input)
				var toolResult llm.ToolResult
				if toolErr != nil {
					toolResult = llm.ToolResult{
						ToolName: block.ToolUse.Name,
						ID:       block.ToolUse.ID,
						IsError:  true,
						Content:  toolErr.Error(),
					}
				} else {
					toolResult = llm.ToolResult{
						ToolName: block.ToolUse.Name,
						ID:       block.ToolUse.ID,
						IsError:  false,
						Content:  toolResp,
					}
				}

				inputMessages = append(inputMessages, llm.Message{
					Type:       llm.MessageTypeToolResult,
					ToolResult: &toolResult,
				})
				fmt.Println("User: ToolResult of ID: " + toolResult.ID + ", of length " + fmt.Sprintf("%d", len(toolResult.Content)))
				//fmt.Println("User: ToolResult: " + toolResult.Content)
			}
		}

		if !hasToolUse {
			break
		}

	}
}
