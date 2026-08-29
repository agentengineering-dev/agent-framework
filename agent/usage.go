package agent

import (
	"time"

	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/gorilla/websocket"
)

// usageTracker accumulates what the framework observes across a session: the
// context the model is carrying, how fast it is producing tokens, and what it
// has cost so far. Every inference folds into it and produces a Usage snapshot
// the ui renders live.
type usageTracker struct {
	provider string
	started  time.Time

	model         string
	contextTokens int64
	contextWindow int64

	inferences    int
	inferenceTime time.Duration
	cost          float64
	totalInput    int64
	totalOutput   int64
	lastTokensSec float64
}

func newUsageTracker(provider string) *usageTracker {
	return &usageTracker{
		provider: provider,
		started:  time.Now(),
	}
}

// record folds one inference of the agent conversation into the session totals
// and returns the snapshot to send to the client. A nil metadata (a provider
// that failed before it told us anything, or a plain end of session refresh)
// still yields a snapshot, so the ui keeps showing the totals it had.
func (u *usageTracker) record(md *llm.InferenceMetadata) Usage {
	return u.fold(md, true)
}

// recordAside folds a one shot inference that is not part of the agent
// conversation, naming the session for instance. It counts towards the
// session totals but its tiny prompt does not move the context gauge.
func (u *usageTracker) recordAside(md *llm.InferenceMetadata) Usage {
	return u.fold(md, false)
}

func (u *usageTracker) fold(md *llm.InferenceMetadata, inConversation bool) Usage {
	if md != nil {
		u.inferences++
		u.inferenceTime += md.Time
		u.cost += md.Cost
		u.totalInput += md.InputTokens
		u.totalOutput += md.OutputTokens
		u.lastTokensSec = md.TokensPerSecond()

		if md.Model != "" {
			u.model = md.Model
		}
		if md.ContextWindow > 0 {
			u.contextWindow = md.ContextWindow
		}
		// the prompt we just sent plus what the model wrote back into it is
		// the conversation the next inference will carry.
		if ctx := md.ContextTokens(); inConversation && ctx > 0 {
			u.contextTokens = ctx
		}
	}

	usage := Usage{
		Provider:        u.provider,
		Model:           u.model,
		ContextTokens:   u.contextTokens,
		ContextWindow:   u.contextWindow,
		TokensPerSecond: u.lastTokensSec,
		Inferences:      u.inferences,
		InputTokens:     u.totalInput,
		OutputTokens:    u.totalOutput,
		Cost:            u.cost,
		InferenceTime:   u.inferenceTime.Seconds(),
		ElapsedTime:     time.Since(u.started).Seconds(),
	}

	if md != nil {
		usage.LastInputTokens = md.InputTokens
		usage.LastOutputTokens = md.OutputTokens
		usage.LastCachedTokens = md.CachedTokens
		usage.LastInferenceTime = md.Time.Seconds()
	}

	if u.contextWindow > 0 {
		usage.ContextPercent = float64(u.contextTokens) / float64(u.contextWindow) * 100
	}

	// throughput over every inference in the session, steadier than the last
	// one on its own.
	if u.inferenceTime > 0 && u.totalOutput > 0 {
		usage.AvgTokensPerSecond = float64(u.totalOutput) / u.inferenceTime.Seconds()
	}

	return usage
}

// sendUsage reports an inference of the agent conversation to the client.
func sendUsage(conn *websocket.Conn, u *usageTracker, md *llm.InferenceMetadata) error {
	return sendUsageSnapshot(conn, u.record(md))
}

// sendUsageAside is sendUsage for an inference outside the agent conversation.
func sendUsageAside(conn *websocket.Conn, u *usageTracker, md *llm.InferenceMetadata) error {
	return sendUsageSnapshot(conn, u.recordAside(md))
}

func sendUsageSnapshot(conn *websocket.Conn, usage Usage) error {
	return sendEvent(conn, AgentSessionEvent{
		Type:  AgentSessionEventUsage,
		Role:  RoleAgent,
		Usage: &usage,
	})
}

// sendStatus tells the client what the agent is busy with, so it can show a
// live timer instead of going quiet during a long inference.
func sendStatus(conn *websocket.Conn, state AgentState, text string) error {
	return sendEvent(conn, AgentSessionEvent{
		Type:  AgentSessionEventStatus,
		Role:  RoleAgent,
		State: state,
		Text:  text,
	})
}
