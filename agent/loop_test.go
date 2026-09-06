package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentengineering.dev/agent-framework/llm"
	"github.com/gorilla/websocket"
)

// scriptedLLM replays a response per inference call and remembers what the
// model was actually shown.
type scriptedLLM struct {
	responses []llm.Message // consumed one text/tool_use message per call
	mds       []*llm.InferenceMetadata
	errs      []error
	seen      [][]llm.Message
}

func (s *scriptedLLM) RunInference(messages []llm.Message, _ []llm.ToolDefinition) ([]llm.Message, *llm.InferenceMetadata, error) {
	s.seen = append(s.seen, messages)
	i := len(s.seen) - 1
	if i >= len(s.responses) {
		return nil, nil, errors.New("script exhausted")
	}
	if i < len(s.errs) && s.errs[i] != nil {
		return nil, s.mds[i], s.errs[i]
	}
	return []llm.Message{s.responses[i]}, s.mds[i], nil
}

func (s *scriptedLLM) GenerateStructuredResponse([]llm.Message, interface{}) (*llm.InferenceMetadata, error) {
	return nil, errors.New("not used")
}

// loopHarness runs runAgentLoop against a scripted llm over a real websocket
// pair and collects every event the loop sends.
func loopHarness(t *testing.T, client llm.LLM, tools map[string]llm.ToolDefinition) <-chan AgentSessionEvent {
	t.Helper()
	events := make(chan AgentSessionEvent, 64)

	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		usage := newUsageTracker("test")
		handoff, err := runAgentLoop(conn, client, []llm.Message{
			{Role: llm.RoleUser, Type: llm.MessageTypeText, Text: "goal"},
		}, tools, usage)
		if err != nil {
			_ = sendText(conn, RoleAgent, "Error: "+err.Error())
		}
		if handoff != nil {
			_ = sendEvent(conn, AgentSessionEvent{Type: AgentSessionEventHandoff, Handoff: handoff})
		}
	}))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	// the reader owns the close: the handler returning drops the connection,
	// which ends this loop, which closes the channel — no send after close.
	go func() {
		defer close(events)
		for {
			var evt AgentSessionEvent
			if err := conn.ReadJSON(&evt); err != nil {
				return
			}
			events <- evt
		}
	}()

	return events
}

func collect(t *testing.T, events <-chan AgentSessionEvent) []AgentSessionEvent {
	t.Helper()
	var out []AgentSessionEvent
	deadline := time.After(5 * time.Second)
	for {
		select {
		case evt, ok := <-events:
			if !ok {
				return out
			}
			out = append(out, evt)
		case <-deadline:
			t.Fatal("timed out waiting for the loop to finish")
		}
	}
}

func text(t string) llm.Message { return llm.Message{Type: llm.MessageTypeText, Text: t} }
func toolUse(name string, input string) llm.Message {
	return llm.Message{Type: llm.MessageTypeToolUse, ToolUse: &llm.ToolUse{ID: "id_" + name, Name: name, Input: json.RawMessage(input)}}
}

func mdOf(ctx, out int64) *llm.InferenceMetadata {
	return &llm.InferenceMetadata{
		Provider: "test", Model: "m", Time: time.Millisecond,
		InputTokens: ctx - out, OutputTokens: out, ContextWindow: 1000,
	}
}

// a whole-file read that would blow past the withhold mark must reach the
// model as a withheld note, never as the content itself, and the session must
// still come back with a handoff.
func TestLoopWithholdsOversizedToolResult(t *testing.T) {
	big := strings.Repeat("x", 6000) // ~2000 estimated tokens on a 1000 window

	fake := &scriptedLLM{
		responses: []llm.Message{
			toolUse("big_read", "{}"),
			toolUse(HandoffToolName, `{"answer":"partial answer","next_prompt":"carry on"}`),
		},
		mds: []*llm.InferenceMetadata{mdOf(400, 10), mdOf(0, 0)},
	}
	fake.mds[1].ContextWindow = 1000
	// the second call has no meaningful usage, keep the model's view
	fake.mds[1].InputTokens, fake.mds[1].OutputTokens = 0, 0

	tools := map[string]llm.ToolDefinition{
		"big_read": {Name: "big_read", Func: func(json.RawMessage) (string, error) { return big, nil }},
	}

	events := collect(t, loopHarness(t, fake, tools))

	var sawWithheldNote bool
	var handoff *Handoff
	for _, evt := range events {
		if evt.Type == AgentSessionEventToolResult && evt.ToolResult != nil &&
			strings.Contains(evt.ToolResult.Content, "withheld from the model") {
			sawWithheldNote = true
		}
		if evt.Type == AgentSessionEventHandoff {
			handoff = evt.Handoff
		}
	}
	if !sawWithheldNote {
		t.Fatal("expected the ui to be told the result was withheld")
	}
	if handoff == nil {
		t.Fatal("expected the session to end in a handoff")
	}
	if handoff.Answer != "partial answer" {
		t.Fatalf("handoff answer = %q", handoff.Answer)
	}

	// the decisive check: what the model was fed on its last call
	last := fake.seen[len(fake.seen)-1]
	for _, m := range last {
		if m.ToolResult != nil {
			if strings.Contains(m.ToolResult.Content, "xxxxx") {
				t.Fatal("the oversized result reached the model")
			}
			if !strings.Contains(m.ToolResult.Content, "Tool result withheld") {
				t.Fatalf("model was fed %q, wanted the withheld note", truncate(string(m.ToolResult.Content), 80))
			}
		}
	}
}

// after the handoff ask every other tool call is rejected without running.
func TestLoopRejectsToolCallsAfterHandoffAsk(t *testing.T) {
	ran := 0
	tools := map[string]llm.ToolDefinition{
		"big_read": {Name: "big_read", Func: func(json.RawMessage) (string, error) {
			ran++
			return "small", nil
		}},
	}

	fake := &scriptedLLM{
		responses: []llm.Message{
			toolUse("big_read", "{}"),
			// 90% of the window: the handoff ask fires, but the model calls a
			// tool again instead of wrapping up
			toolUse("big_read", "{}"),
			toolUse(HandoffToolName, `{"answer":"done enough"}`),
		},
		mds: []*llm.InferenceMetadata{mdOf(400, 10), mdOf(900, 10), mdOf(0, 0)},
	}

	events := collect(t, loopHarness(t, fake, tools))

	if ran != 1 {
		t.Fatalf("tool ran %d times, want exactly once (before the handoff ask)", ran)
	}

	var sawRejection bool
	for _, evt := range events {
		if evt.Type == AgentSessionEventToolResult && evt.ToolResult != nil &&
			strings.Contains(evt.ToolResult.Content, "rejected by the agent") {
			sawRejection = true
		}
	}
	if !sawRejection {
		t.Fatal("expected a rejection event for the tool call after the handoff ask")
	}
}

// a model that runs out of tokens before writing anything still yields a
// continuable handoff instead of a dead session.
func TestLoopMaxTokensWithoutAnswersStillHandsOff(t *testing.T) {
	fake := &scriptedLLM{
		responses: []llm.Message{toolUse("big_read", "{}")},
		mds:       []*llm.InferenceMetadata{mdOf(400, 10)},
		errs:      []error{nil, errors.New("max token exceeded")},
	}
	// the model called a tool, then the next inference died
	fake.responses = append(fake.responses, llm.Message{})
	fake.mds = append(fake.mds, mdOf(0, 0))
	fake.errs = append(fake.errs, errors.New("max token exceeded"))

	tools := map[string]llm.ToolDefinition{
		"big_read": {Name: "big_read", Func: func(json.RawMessage) (string, error) { return "fine", nil }},
	}

	events := collect(t, loopHarness(t, fake, tools))

	var handoff *Handoff
	for _, evt := range events {
		if evt.Type == AgentSessionEventHandoff {
			handoff = evt.Handoff
		}
	}
	if handoff == nil {
		t.Fatal("expected a recovery handoff, got a dead session")
	}
	if !strings.Contains(handoff.Answer, "ran out of context") {
		t.Fatalf("recovery answer = %q", truncate(handoff.Answer, 80))
	}
}

// the handoff tool must not work before the ask, or a model could end its
// own session whenever it feels like stopping.
func TestLoopBouncesEarlyHandoffCall(t *testing.T) {
	fake := &scriptedLLM{
		responses: []llm.Message{
			toolUse(HandoffToolName, `{"answer":"lazy"}`),
			text("fine, continuing"),
		},
		mds: []*llm.InferenceMetadata{mdOf(100, 10), mdOf(150, 20)},
	}

	events := collect(t, loopHarness(t, fake, nil))

	var bounced bool
	var handoff *Handoff
	for _, evt := range events {
		if evt.Type == AgentSessionEventToolResult && evt.ToolResult != nil &&
			strings.Contains(evt.ToolResult.Content, "bounced") {
			bounced = true
		}
		if evt.Type == AgentSessionEventHandoff {
			handoff = evt.Handoff
		}
	}
	if !bounced {
		t.Fatal("expected the early handoff call to be bounced")
	}
	if handoff != nil {
		t.Fatal("session ended via handoff without the ask")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
