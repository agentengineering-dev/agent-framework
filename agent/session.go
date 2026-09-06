package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/agentengineering.dev/agent-framework/tool"
	"github.com/gin-gonic/gin"
)

const titlePrompt = "Given the goal below, give a short title summarizing the session: Given goal\n "

// CreateAgentSession runs the agent loop against the working directory as it
// is. It does no git work, the only inference it makes on top of the loop is
// an ai generated title for the session.
func CreateAgentSession(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// read initial request message
	params, err := readSessionParams(conn)
	if err != nil {
		sendError(conn, err)
		return
	}

	err = sendText(conn, RoleAgent, "Agent Started: ")
	if err != nil {
		return
	}

	client, err := llm.NewClient(params.Provider)
	if err != nil {
		sendError(conn, err)
		return
	}

	err = sendText(conn, RoleUser, params.Goal)
	if err != nil {
		return
	}

	usage := newUsageTracker(params.Provider)

	// agent loop
	inputMessages := []llm.Message{
		{
			Role: llm.RoleUser,
			Text: systemPrompt,
			Type: llm.MessageTypeText,
		},
		{
			Role: llm.RoleUser,
			Text: params.Goal,
			Type: llm.MessageTypeText,
		},
	}

	// a continued session seeds the previous session's handoff instead of
	// spending a fresh inference on a title it already has.
	if params.Resume != nil {
		preamble := resumePreamble(params.Resume)
		inputMessages = append(inputMessages, llm.Message{
			Role: llm.RoleUser,
			Text: preamble,
			Type: llm.MessageTypeText,
		})

		title := params.Resume.Title
		if title == "" {
			title = "Continued session"
		}
		if !strings.HasPrefix(title, "Continued: ") {
			title = "Continued: " + title
		}

		if err := sendText(conn, RoleAgent, "Continuing from a previous session that ran low on context — the original goal, its partial answer and the prompt for this run were seeded into the fresh conversation."); err != nil {
			return
		}
		if err := sendEvent(conn, AgentSessionEvent{
			Type: AgentSessionEventTitle,
			Role: RoleAgent,
			Text: title,
		}); err != nil {
			return
		}
	} else {
		if err := sendStatus(conn, AgentStateInferring, "naming the session"); err != nil {
			return
		}

		// name the session
		sessionTitle := SessionTitle{}
		md, err := client.GenerateStructuredResponse([]llm.Message{
			{
				Role: llm.RoleUser,
				Type: llm.MessageTypeText,
				Text: titlePrompt + params.Goal,
			},
		}, &sessionTitle)
		if err != nil {
			sendError(conn, err)
			return
		}

		fmt.Println(fmt.Sprintf("Agent: Inference cost: $%f, time: %s", md.Cost, md.Time))

		if err := sendUsageAside(conn, usage, md); err != nil {
			return
		}

		err = sendEvent(conn, AgentSessionEvent{
			Type: AgentSessionEventTitle,
			Role: RoleAgent,
			Text: sessionTitle.Title,
		})
		if err != nil {
			return
		}
	}

	handoff, err := runAgentLoop(conn, client, inputMessages, tool.ToolMap, usage)
	if err != nil {
		_ = sendStatus(conn, AgentStateError, err.Error())
		sendError(conn, err)
		return
	}

	if handoff != nil {
		handoff.Goal = params.Goal
		if err := sendEvent(conn, AgentSessionEvent{
			Type:    AgentSessionEventHandoff,
			Role:    RoleAgent,
			Handoff: handoff,
		}); err != nil {
			return
		}
		if err := sendStatus(conn, AgentStateDone, "handed off — continue in a fresh session when ready"); err != nil {
			return
		}
		return
	}

	final := usage.record(nil)
	result := fmt.Sprintf("Agent Session Ended: Session Inference cost: $%f, time: %s", final.Cost, time.Duration(final.InferenceTime*float64(time.Second)))
	fmt.Println(result)

	if err := sendStatus(conn, AgentStateDone, result); err != nil {
		return
	}

	err = sendText(conn, RoleAgent, result)
	if err != nil {
		return
	}
}

// resumePreamble is the message that drops a previous session's handoff into
// a fresh conversation.
func resumePreamble(resume *SessionResume) string {
	preamble := "This session continues a previous one that ran low on context. The original goal is repeated above.\n\nThe previous session's answer so far:\n" + resume.Answer + "\n"
	if strings.TrimSpace(resume.NextPrompt) != "" {
		preamble += "\nThe previous session's prompt for this run:\n" + resume.NextPrompt + "\n"
	} else {
		preamble += "\nThe previous session left no explicit prompt; continue its work from the answer above.\n"
	}
	return preamble + "\nPick up from there. The context window is fresh, but be economical with it."
}
