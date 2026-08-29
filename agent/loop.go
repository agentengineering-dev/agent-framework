package agent

import (
	"encoding/json"
	"fmt"

	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/agentengineering.dev/agent-framework/tool"
	"github.com/gorilla/websocket"
)

const systemPrompt = `
You are an autonomous agent working in a project repository.
The goal can either be an question or instruction.
If it's a question, append the answer to QA.md file 
Else follow the given instruction 
Follow the goal given below:
`

// toolDefinitions flattens a tool set into the list the llm client expects.
func toolDefinitions(tools map[string]llm.ToolDefinition) []llm.ToolDefinition {
	defs := []llm.ToolDefinition{}
	for _, t := range tools {
		defs = append(defs, t)
	}
	return defs
}

func sendEvent(conn *websocket.Conn, event AgentSessionEvent) error {
	return conn.WriteJSON(event)
}

func sendText(conn *websocket.Conn, role Role, text string) error {
	return sendEvent(conn, AgentSessionEvent{
		Type: AgentSessionEventText,
		Role: role,
		Text: text,
	})
}

// sendError reports a failure to the client without killing the process, the
// session is over either way so a write failure here is not worth handling.
func sendError(conn *websocket.Conn, err error) {
	fmt.Println("Agent: error: " + err.Error())
	_ = sendText(conn, RoleAgent, "Error: "+err.Error())
}

// readSessionParams reads the initial request message off the socket.
func readSessionParams(conn *websocket.Conn) (*CreateAgentSessionParams, error) {
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read request: %w", err)
	}

	var params CreateAgentSessionParams
	if err := json.Unmarshal(msg, &params); err != nil {
		return nil, err
	}
	return &params, nil
}

// runAgentLoop runs inference and executes the requested tools until the model
// answers without asking for another tool. Every inference is folded into the
// usage tracker, which is what the ui reports context and throughput from.
func runAgentLoop(conn *websocket.Conn, client llm.LLM, inputMessages []llm.Message, tools map[string]llm.ToolDefinition, usage *usageTracker) error {
	toolDefs := toolDefinitions(tools)

	for {
		if err := sendStatus(conn, AgentStateInferring, "running inference"); err != nil {
			return err
		}

		// run inference.
		respMessage, md, err := client.RunInference(inputMessages, toolDefs)
		if err != nil {
			// the provider may still have told us what the failed call cost,
			// report it before giving up on the session.
			_ = sendUsage(conn, usage, md)
			return err
		}

		if err := sendUsage(conn, usage, md); err != nil {
			return err
		}

		fmt.Println(fmt.Sprintf("Agent: Loop Inference cost: $%f, time: %s, context: %d tokens, %.1f tok/s", md.Cost, md.Time, md.ContextTokens(), md.TokensPerSecond()))

		// print the response
		for _, message := range respMessage {
			if message.Text != "" {
				fmt.Println("Assistant: " + message.Text)
				err := sendText(conn, RoleAssistant, message.Text)
				if err != nil {
					return err
				}
			} else if message.ToolUse != nil {
				inputJson, _ := json.MarshalIndent(message.ToolUse.Input, "", "  ")
				fmt.Println(fmt.Sprintf("Assistant: ToolUse: ID: %s, Name: %s, Input: %s", message.ToolUse.ID, message.ToolUse.Name, string(inputJson)))
				err := sendEvent(conn, AgentSessionEvent{
					Type: AgentSessionEventToolUse,
					Role: RoleAssistant,
					ToolUse: &ToolUse{
						ID:    message.ToolUse.ID,
						Name:  message.ToolUse.Name,
						Input: message.ToolUse.Input,
					},
				})
				if err != nil {
					return err
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

				if err := sendStatus(conn, AgentStateTool, "running "+block.ToolUse.Name); err != nil {
					return err
				}

				toolResp, toolErr := tool.Execute(tools, block.ToolUse.Name, block.ToolUse.Input)
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

				err := sendEvent(conn, AgentSessionEvent{
					Type: AgentSessionEventToolResult,
					Role: RoleAssistant,
					ToolResult: &ToolResult{
						ID:       block.ToolUse.ID,
						ToolName: block.ToolUse.Name,
						// toolResp is empty when the tool failed, the reason is
						// on the result we are about to feed back to the model.
						Content: toolResult.Content,
						IsError: toolResult.IsError,
					},
				})
				if err != nil {
					return err
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
			return nil
		}
	}
}
