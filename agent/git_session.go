package agent

import (
	"fmt"
	"time"

	"github.com/agentengineering.dev/agent-framework/git_helpers"
	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/agentengineering.dev/agent-framework/tool"
	"github.com/gin-gonic/gin"
)

const branchPrompt = "Given the goal below, give a name summarizing the session, and name of the branch that needs to be created in the repo: Given goal\n "

// CreateGitAgentSession runs the agent loop on a branch of its own, the branch
// name is picked by the model from the goal.
func CreateGitAgentSession(c *gin.Context) {
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

	if err := sendStatus(conn, AgentStateInferring, "naming the branch"); err != nil {
		return
	}

	// git init.
	git_helpers.Init()

	// make llm inference for name?
	sessionMetadata := SessionMetadata{}
	md, err := client.GenerateStructuredResponse([]llm.Message{
		{
			Role: llm.RoleUser,
			Type: llm.MessageTypeText,
			Text: branchPrompt + params.Goal,
		},
	}, &sessionMetadata)
	if err != nil {
		sendError(conn, err)
		return
	}

	fmt.Println(fmt.Sprintf("Agent: Inference cost: $%f, time: %s", md.Cost, md.Time))

	if err := sendUsageAside(conn, usage, md); err != nil {
		return
	}

	branch := sessionMetadata.BranchName

	err = sendText(conn, RoleAgent, "Branch created: "+branch)
	if err != nil {
		return
	}

	// create a branch
	err = git_helpers.CreateBranch(branch)
	if err != nil {
		sendError(conn, fmt.Errorf("failed to create branch: %w", err))
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

	err = runAgentLoop(conn, client, inputMessages, tool.GitToolMap, usage)
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
