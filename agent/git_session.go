package agent

import (
	"fmt"

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

	err = sendText(conn, RoleUser, params.Provider)
	if err != nil {
		return
	}

	err = sendText(conn, RoleUser, params.Goal)
	if err != nil {
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

	cost, totalTime, err := runAgentLoop(conn, client, inputMessages, tool.GitToolMap)

	// the branch naming inference is part of the session too
	cost += md.Cost
	totalTime += md.Time

	if err != nil {
		sendError(conn, err)
		return
	}

	result := fmt.Sprintf("Agent Session Ended: Session Inference cost: $%f, time: %s", cost, totalTime)
	fmt.Println(result)

	err = sendText(conn, RoleAgent, result)
	if err != nil {
		return
	}
}
