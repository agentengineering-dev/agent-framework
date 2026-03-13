package agent

import (
	"encoding/json"
	"fmt"
	"github.com/agentengineering.dev/agent-framework/git_helpers"
	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/agentengineering.dev/agent-framework/tool"
	"github.com/gin-gonic/gin"
	"log"
	"time"
)

const systemPrompt = `
You are an autonomous agent working in a project repository.
The goal can either be an question or instruction.
If it's a question, append the answer to QA.md file 
Else follow the given instruction 
Follow the goal given below:
`

func CreateAgentSession(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// read initial request message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		conn.WriteJSON(gin.H{"error": "failed to read request"})
		return
	}
	event := AgentSessionEvent{
		Type: AgentSessionEventText,
		Role: RoleAgent,
		Text: "Agent Started: ",
	}

	err = conn.WriteJSON(event)
	if err != nil {
		return
	}

	var r CreateAgentSessionParams
	if err := json.Unmarshal(msg, &r); err != nil {
		conn.WriteJSON(gin.H{"error": err.Error()})
		return
	}

	client, err := llm.NewClient(r.Provider)
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
			Text: r.Goal,
			Type: llm.MessageTypeText,
		},
	}
	event = AgentSessionEvent{
		Type: AgentSessionEventText,
		Role: RoleUser,
		Text: r.Provider,
	}

	err = conn.WriteJSON(event)
	if err != nil {
		return
	}

	event = AgentSessionEvent{
		Type: AgentSessionEventText,
		Role: RoleUser,
		Text: r.Goal,
	}

	err = conn.WriteJSON(event)
	if err != nil {
		return
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

	sessionInitMessages := []llm.Message{
		{
			Role: llm.RoleUser,
			Type: llm.MessageTypeText,
			Text: "Given the goal below, give a name summarizing the session, and name of the branch that needs to be created in the repo: Given goal\n " + r.Goal,
		},
	}

	sessionMetadata := SessionMetadata{}

	md, err := client.GenerateStructuredResponse(sessionInitMessages, &sessionMetadata)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(fmt.Sprintf("Agent: Inference cost: $%f, time: %s", md.Cost, md.Time))

	branch := sessionMetadata.BranchName

	event = AgentSessionEvent{
		Type: AgentSessionEventText,
		Role: RoleAgent,
		Text: "Branch created: " + branch,
	}

	err = conn.WriteJSON(event)
	if err != nil {
		return
	}

	// create a branch
	err = git_helpers.CreateBranch(branch)
	if err != nil {
		fmt.Println("failed to create branch: ", err.Error())
		return
	}
	cost := 0.0
	totalTime := time.Duration(0)
	for {
		// run inference.
		respMessage, md, err := client.RunInference(inputMessages, allTools)
		if err != nil {
			log.Fatal(err)
		}

		cost += float64(md.Cost)
		totalTime += md.Time

		fmt.Println(fmt.Sprintf("Agent: Loop Inference cost: $%f, time: %s", md.Cost, md.Time))

		// print the response
		for _, message := range respMessage {
			if message.Text != "" {
				fmt.Println("Assistant: " + message.Text)
				event := AgentSessionEvent{
					Type: AgentSessionEventText,
					Role: RoleAssistant,
					Text: message.Text,
				}

				err := conn.WriteJSON(event)
				if err != nil {
					return
				}

			} else if message.ToolUse != nil {
				inputJson, _ := json.MarshalIndent(message.ToolUse.Input, "", "  ")
				fmt.Println(fmt.Sprintf("Assistant: ToolUse: ID: %s, Name: %s, Input: %s", message.ToolUse.ID, message.ToolUse.Name, string(inputJson)))
				event := AgentSessionEvent{
					Type: AgentSessionEventToolUse,
					Role: RoleAssistant,
					ToolUse: &ToolUse{
						ID:    message.ToolUse.ID,
						Name:  message.ToolUse.Name,
						Input: message.ToolUse.Input,
					},
				}

				err := conn.WriteJSON(event)
				if err != nil {
					return
				}
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
				event := AgentSessionEvent{
					Type: AgentSessionEventToolResult,
					Role: RoleAssistant,
					ToolResult: &ToolResult{
						ID:       block.ToolUse.ID,
						ToolName: block.ToolUse.Name,
						Content:  toolResp,
						IsError:  toolResult.IsError,
					},
				}

				err := conn.WriteJSON(event)
				if err != nil {
					return
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
	result := fmt.Sprintf("Agent Session Ended: Session Inference cost: $%f, time: %s", md.Cost, md.Time)
	fmt.Println(result)
	event = AgentSessionEvent{
		Type: AgentSessionEventText,
		Role: RoleAgent,
		Text: result,
	}

	err = conn.WriteJSON(event)
	if err != nil {
		return
	}
}
