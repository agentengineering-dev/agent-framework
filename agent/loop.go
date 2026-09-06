package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/agentengineering.dev/agent-framework/tool"
	"github.com/gorilla/websocket"
)

// Context thresholds for the handoff flow, as a share of the model's window.
// They run on an estimate that includes tool results appended since the last
// inference, and they sit low on purpose: the model needs one more inference
// (plus its output, thinking tokens included) after the handoff ask, and a
// single whole-file read can move the gauge by tens of percent.
const (
	contextHintPercent    = 50.0
	contextHandoffPercent = 70.0
	// a tool result estimated to push the conversation past this share of the
	// window is withheld from the model; the tool still runs.
	contextWithholdPercent = 80.0
	// rough characters per token for estimating result sizes. undercounting
	// tokens is what overflows windows, so this errs high.
	charsPerToken = 3
)

// HandoffToolName is the tool the model calls to hand its work to a fresh
// session. It is offered on every inference; the loop only honours it once
// the handoff threshold has been crossed, before that a call is bounced back
// as an error so the model keeps working.
const HandoffToolName = "write_handoff"

// handoffInput is the schema of the handoff tool call.
type handoffInput struct {
	Answer     string `json:"answer" jsonschema_description:"The answer to the goal as far as the work has gotten: findings, decisions and what was changed, everything a fresh session needs without redoing the work"`
	NextPrompt string `json:"next_prompt" jsonschema_description:"A precise prompt for a fresh session with the same goal: what is already done and what to do next"`
}

var handoffToolDef = llm.ToolDefinition{
	Name:                HandoffToolName,
	Description:         "Write down a handoff so a fresh session can continue this work: your answer so far and a prompt for the next run. Only call this when the instructions tell you to.",
	InputSchemaInstance: &handoffInput{},
}

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
//
// As the conversation fills the model's window the loop steers toward a
// handoff: at contextHintPercent the model is nudged to be economical, at
// contextHandoffPercent it is told to call write_handoff with its answer so
// far and a prompt for a fresh session. It returns a handoff when the model
// wrote one (or when it ran out of tokens mid answer, in which case the last
// answer stands in for one), and nil when the session simply finished.
func runAgentLoop(conn *websocket.Conn, client llm.LLM, inputMessages []llm.Message, tools map[string]llm.ToolDefinition, usage *usageTracker) (*Handoff, error) {
	toolDefs := append(toolDefinitions(tools), handoffToolDef)

	hinted := false
	handoffAsked := false
	// every answer the model gave, the fallback handoff when it runs out of
	// tokens without writing one down.
	answers := []string{}
	// context estimate including what has been appended since the last
	// inference. the reported gauge lags behind by exactly the tool results
	// gathered in the current round, and that lag is what overflows windows.
	ctxEstimate := int64(0)

	for {
		if err := sendStatus(conn, AgentStateInferring, "running inference"); err != nil {
			return nil, err
		}

		// run inference.
		respMessage, md, err := client.RunInference(inputMessages, toolDefs)
		if err != nil {
			// the provider may still have told us what the failed call cost,
			// report it before giving up on the session.
			_, _ = sendUsage(conn, usage, md)
			// a model that hit its output limit mid answer has still done
			// work worth continuing from.
			if isMaxTokenErr(err) {
				fmt.Println("Agent: max tokens exceeded, falling back to a handoff")
				answer := strings.TrimSpace(strings.Join(answers, "\n\n"))
				if answer == "" {
					// nothing written down, the fresh session has to re-explore.
					// still better than a dead session: the goal and tools are
					// known, only the work is lost.
					answer = "The session ran out of context before an answer was written down. It had been working with the tools listed in the conversation."
				}
				return &Handoff{
					Answer:     clampAnswer(answer),
					NextPrompt: "The previous session was cut off by the context limit. Review what it wrote, then finish the remaining work.",
				}, nil
			}
			return nil, err
		}

		snap, err := sendUsage(conn, usage, md)
		if err != nil {
			return nil, err
		}
		ctxEstimate = md.ContextTokens()

		fmt.Println(fmt.Sprintf("Agent: Loop Inference cost: $%f, time: %s, context: %d tokens, %.1f tok/s", md.Cost, md.Time, md.ContextTokens(), md.TokensPerSecond()))

		// print the response
		for _, message := range respMessage {
			if message.Text != "" {
				fmt.Println("Assistant: " + message.Text)
				answers = append(answers, message.Text)
				err := sendText(conn, RoleAssistant, message.Text)
				if err != nil {
					return nil, err
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
					return nil, err
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

				if block.ToolUse.Name == HandoffToolName {
					if !handoffAsked {
						// the model jumped the gun, the session is not out of
						// context yet. bounce it back so it keeps working.
						fmt.Println("Agent: " + HandoffToolName + " called before it was asked for, bouncing")
						inputMessages = append(inputMessages, llm.Message{
							Type: llm.MessageTypeToolResult,
							ToolResult: &llm.ToolResult{
								ID:       block.ToolUse.ID,
								ToolName: block.ToolUse.Name,
								IsError:  true,
								Content:  HandoffToolName + " is not needed yet, the context is still fine. Keep working on the goal.",
							},
						})
						if err := sendEvent(conn, AgentSessionEvent{
							Type: AgentSessionEventToolResult,
							Role: RoleAssistant,
							ToolResult: &ToolResult{
								ID:       block.ToolUse.ID,
								ToolName: block.ToolUse.Name,
								Content:  "bounced: the handoff was not asked for yet, keep working",
								IsError:  true,
							},
						}); err != nil {
							return nil, err
						}
						continue
					}

					var written handoffInput
					if err := json.Unmarshal(block.ToolUse.Input, &written); err != nil {
						written = handoffInput{Answer: string(block.ToolUse.Input)}
					}
					return &Handoff{
						Answer:     clampAnswer(written.Answer),
						NextPrompt: strings.TrimSpace(written.NextPrompt),
					}, nil
				}

				// past the handoff mark no tool runs: the window is nearly
				// full and executing more calls is how sessions overflow.
				// the check covers both the asked-for case and the round in
				// which the estimate first crosses the mark, whose ask has
				// not been appended yet — that call is the one that overflows.
				overMark := usage.contextWindow > 0 &&
					float64(ctxEstimate)/float64(usage.contextWindow)*100 >= contextHandoffPercent
				if handoffAsked || overMark {
					fmt.Println("Agent: " + block.ToolUse.Name + " rejected, handoff already asked for")
					inputMessages = append(inputMessages, llm.Message{
						Type: llm.MessageTypeToolResult,
						ToolResult: &llm.ToolResult{
							ID:       block.ToolUse.ID,
							ToolName: block.ToolUse.Name,
							IsError:  true,
							Content:  "Rejected: the context window is nearly full. Do not run more tools. Call " + HandoffToolName + " now.",
						},
					})
					if err := sendEvent(conn, AgentSessionEvent{
						Type: AgentSessionEventToolResult,
						Role: RoleAssistant,
						ToolResult: &ToolResult{
							ID:       block.ToolUse.ID,
							ToolName: block.ToolUse.Name,
							Content:  "rejected by the agent: context nearly full, the handoff ask stands",
							IsError:  true,
						},
					}); err != nil {
						return nil, err
					}
					continue
				}

				if err := sendStatus(conn, AgentStateTool, "running "+block.ToolUse.Name); err != nil {
					return nil, err
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

				// a tool result big enough to push the conversation past the
				// withhold mark is kept from the model. the tool ran and the
				// ui shows what it returned, but feeding the whole thing back
				// is what kills the next inference.
				if withheld, note := withholdResult(usage, ctxEstimate, toolResult.Content); withheld {
					fmt.Println("Agent: result of " + block.ToolUse.Name + " withheld from the model: " + note)
					// the withheld note tells the model to hand off, so from
					// here the loop honours that call.
					handoffAsked = true
					inputMessages = append(inputMessages, llm.Message{
						Type: llm.MessageTypeToolResult,
						ToolResult: &llm.ToolResult{
							ID:       block.ToolUse.ID,
							ToolName: block.ToolUse.Name,
							IsError:  true,
							Content:  note,
						},
					})
					ctxEstimate += int64(len(note)) / charsPerToken
					err := sendEvent(conn, AgentSessionEvent{
						Type: AgentSessionEventToolResult,
						Role: RoleAssistant,
						ToolResult: &ToolResult{
							ID:       block.ToolUse.ID,
							ToolName: block.ToolUse.Name,
							Content:  toolResult.Content + "\n\n[withheld from the model: " + note + "]",
							IsError:  false,
						},
					})
					if err != nil {
						return nil, err
					}
					continue
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
					return nil, err
				}

				inputMessages = append(inputMessages, llm.Message{
					Type:       llm.MessageTypeToolResult,
					ToolResult: &toolResult,
				})
				ctxEstimate += int64(len(toolResult.Content)) / charsPerToken
				fmt.Println("User: ToolResult of ID: " + toolResult.ID + ", of length " + fmt.Sprintf("%d", len(toolResult.Content)))
				//fmt.Println("User: ToolResult: " + toolResult.Content)
			}
		}

		// context hooks, appended after the tool results so the providers see
		// a well formed turn. they run on the estimate, which includes the
		// results gathered this round — the reported gauge does not. each
		// fires once per session.
		pct := snap.ContextPercent
		if usage.contextWindow > 0 && ctxEstimate > 0 {
			pct = float64(ctxEstimate) / float64(usage.contextWindow) * 100
		}
		if pct >= contextHandoffPercent && !handoffAsked {
			handoffAsked = true
			inputMessages = append(inputMessages, llm.Message{
				Role: llm.RoleUser,
				Type: llm.MessageTypeText,
				Text: handoffInstruction(pct, snap),
			})
			if err := sendText(conn, RoleAgent, "Context "+fmt.Sprintf("%.0f%%", pct)+" used ("+formatTokens(ctxEstimate)+" / "+formatTokens(usage.contextWindow)+") — the model was told to write down its answer and a prompt for the next run."); err != nil {
				return nil, err
			}
		} else if pct >= contextHintPercent && !hinted {
			hinted = true
			inputMessages = append(inputMessages, llm.Message{
				Role: llm.RoleUser,
				Type: llm.MessageTypeText,
				Text: "System note: you have used " + fmt.Sprintf("%.0f%%", pct) + " of the context window (" + formatTokens(ctxEstimate) + " / " + formatTokens(usage.contextWindow) + " tokens). Be economical from here: do not re-read files you have already seen, avoid reading whole files when part of one would do, and start moving toward a result.",
			})
			if err := sendText(conn, RoleAgent, "Context "+fmt.Sprintf("%.0f%%", pct)+" used ("+formatTokens(ctxEstimate)+" / "+formatTokens(usage.contextWindow)+") — the model was nudged to be economical."); err != nil {
				return nil, err
			}
		}

		if !hasToolUse {
			return nil, nil
		}
	}
}

// handoffInstruction is the message that turns the model toward writing its
// handoff instead of carrying on.
func handoffInstruction(pct float64, snap Usage) string {
	return "System note: only " + fmt.Sprintf("%.0f%%", 100-pct) + " of the context window remains. Stop exploring and wrap up now. Call the " + HandoffToolName + " tool, and only that tool, with: answer — everything the goal's reader needs from the work so far, findings, decisions and changes included; next_prompt — a precise prompt for a fresh session with the same goal, saying what is done and what to do next. Any further tool calls will be rejected."
}

// withholdResult decides whether a tool result is too big to feed back to the
// model. It returns the note to hand the model in place of the content.
func withholdResult(usage *usageTracker, ctxEstimate int64, content string) (bool, string) {
	if usage.contextWindow <= 0 || content == "" {
		return false, ""
	}
	projected := ctxEstimate + int64(len(content))/charsPerToken
	if projected <= usage.contextWindow*contextWithholdPercent/100 {
		return false, ""
	}
	note := fmt.Sprintf("Tool result withheld: %d KB would push the conversation past %s of the %s token context window. The tool ran, the content exists but is not shown to you. Call %s now with what you already know.",
		len(content)/1024+1, formatTokens(projected), formatTokens(usage.contextWindow), HandoffToolName)
	return true, note
}

// isMaxTokenErr matches both spellings the providers use, anthropic and
// google say max_tokens, the openai compatible ones say max token.
func isMaxTokenErr(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "max_token") || strings.Contains(err.Error(), "max token"))
}

func formatTokens(n int64) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.2fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// clampAnswer keeps a handoff answer from dominating the fresh session's own
// context. The tail is dropped, findings usually come first.
func clampAnswer(answer string) string {
	const limit = 12000
	answer = strings.TrimSpace(answer)
	if len(answer) <= limit {
		return answer
	}
	return answer[:limit] + "\n\n[answer truncated at " + fmt.Sprintf("%d", limit) + " characters]"
}
