package agent

import (
	"fmt"
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

	err = runAgentLoop(conn, client, inputMessages, tool.ToolMap, usage)
	if err != nil {
		_ = sendStatus(conn, AgentStateError, err.Error())
		sendError(conn, err)
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
