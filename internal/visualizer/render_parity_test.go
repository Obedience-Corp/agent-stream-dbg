package visualizer

import (
	"sort"
	"strings"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

// render_parity_test.go is phase 007's closing gate: it renders the real
// reference session fixture through the current
// (Kind+role-based) visualizer and proves the result is unchanged from
// the pre-phase baseline — commit 62d408f, the last commit before phase
// 007 started — for every event Kind this phase didn't deliberately
// change: session_start, content, stream_start/stream_end,
// step_start/step_end, detail, and error. (This fixture contains no
// agent_metadata event, so the "usage" Kind isn't exercised here.)
// Handoff/ToolCall/ToolResult are intentionally excluded — sequence 04
// built new rendering for those on purpose, so asserting identity there
// would be wrong, not a regression check.
//
// The baseline values below were captured by checking out 62d408f into a
// scratch git worktree and running ITS interactive.go (pre-split
// monolith) against this same fixture through the identical
// applyParsedEvent/render call sequence used here, then diffing that
// output against the current tree's. Two — and only two — differences
// exist, both deliberate: a handful of Go-authored UI labels (an event
// field's display name, a response header, a pane toggle and its hint
// text, an inline metrics prefix) that named the retired identifier this
// whole phase generalizes to "Aggregator" — see staleAggregatorLabel
// below. Purging that retired identifier from Go-authored UI vocabulary
// (not from real dialect data, which is untouched) is this task's whole
// point. Every other value below — numeric fields, structural fields,
// and every string that never named it — is asserted byte-for-byte or
// field-for-field identical to that baseline.
func TestRenderParity_ReferenceFixture(t *testing.T) {
	m := newFixtureModel(t)
	msg := m.messages[0]
	var firstWorker, secondWorker string
	var timelineWorkers []string
	for id, response := range msg.AgentResponses {
		if response.Role == events.RoleAggregator {
			continue
		}
		timelineWorkers = append(timelineWorkers, id)
		if strings.HasPrefix(response.FullContent, "Let's think") {
			firstWorker = id
		} else {
			secondWorker = id
		}
	}
	if firstWorker == "" || secondWorker == "" {
		t.Fatal("expected two non-aggregator responses in the reference fixture")
	}
	sort.Strings(timelineWorkers)
	routingNode := m.buildFlowNodes(msg.Events)["routing"]
	if routingNode == nil {
		t.Fatal("expected a routing flow node")
	}

	t.Run("step_start_step_end_flow_node_fields_unchanged", func(t *testing.T) {
		nodes := m.buildFlowNodes(msg.Events)

		routing := nodes["routing"]
		if routing == nil {
			t.Fatal("expected a routing flow node")
		}
		if !routing.Enabled || routing.AgentCount != 2 || routing.DurationMs != 70 ||
			routing.RoutingMode != "broadcast" || routing.RouteTaken != "all" ||
			strings.Join(routing.RouteAgents, ",") != firstWorker+","+secondWorker {
			t.Errorf("routing flow node fields changed from baseline: %+v", routing)
		}

		synthesis := nodes["synthesis"]
		if synthesis == nil {
			t.Fatal("expected a synthesis flow node")
		}
		if !synthesis.Enabled || synthesis.AgentCount != 2 || synthesis.DurationMs != 560 {
			t.Errorf("synthesis flow node fields changed from baseline: %+v", synthesis)
		}
	})

	t.Run("content_stream_start_stream_end_agent_responses_unchanged", func(t *testing.T) {
		agg := m.aggregatorResponse(msg)
		if agg == nil {
			t.Fatal("expected an aggregator AgentResponse")
		}
		const wantAggContent = "Bringing both viewpoints together, the synthesis suggests a balanced, grounded next step."
		if agg.FullContent != wantAggContent {
			t.Errorf("aggregator FullContent changed from baseline:\n got:  %q\n want: %q", agg.FullContent, wantAggContent)
		}
		if agg.TokenCount != 28 || !agg.Completed || agg.StartTime.IsZero() || agg.EndTime.IsZero() {
			t.Errorf("aggregator metrics changed from baseline: tokens=%d completed=%v startZero=%v endZero=%v",
				agg.TokenCount, agg.Completed, agg.StartTime.IsZero(), agg.EndTime.IsZero())
		}

		wantAgents := map[string]struct {
			content string
			tokens  int
		}{
			firstWorker:  {"Let's think about this together. There are a few threads worth pulling on. Taken together, this points toward a clear next step.", 3},
			secondWorker: {"Here's another angle worth considering. It helps to slow down and notice what's present. Continuing from where we left off, the same idea still holds.", 3},
		}
		for id, want := range wantAgents {
			ar := msg.AgentResponses[id]
			if ar == nil {
				t.Fatalf("expected an AgentResponse for %s", id)
			}
			if ar.FullContent != want.content {
				t.Errorf("%s FullContent changed from baseline:\n got:  %q\n want: %q", id, ar.FullContent, want.content)
			}
			// Non-aggregator lanes never tracked StartTime/EndTime — the
			// asymmetry this task preserved, not introduced (also
			// covered from the applyParsedEvent-state angle by
			// fixture_parity_test.go; here from the render-facing
			// angle).
			if ar.TokenCount != want.tokens || !ar.Completed || !ar.StartTime.IsZero() {
				t.Errorf("%s metrics changed from baseline: tokens=%d completed=%v startZero=%v",
					id, ar.TokenCount, ar.Completed, ar.StartTime.IsZero())
			}
		}
	})

	t.Run("event_expanded_plain_text_unchanged_by_kind", func(t *testing.T) {
		findByName := func(name string) *events.Event {
			for _, evt := range msg.Events {
				if evt.Name == name {
					return evt
				}
			}
			return nil
		}
		findBySourceID := func(name, sourceID string) *events.Event {
			for _, evt := range msg.Events {
				if evt.Name == name && evt.SourceID == sourceID {
					return evt
				}
			}
			return nil
		}
		findByStep := func(name, step string) *events.Event {
			for _, evt := range msg.Events {
				if evt.Name == name && evt.StringField("step") == step {
					return evt
				}
			}
			return nil
		}
		findByKind := func(kind events.Kind) *events.Event {
			return findAggregatorEvent(*m, msg.Events, kind)
		}
		errorEvent := findByName("error")
		errorAgent := ""
		if errorEvent != nil {
			errorAgent = errorEvent.StringField("agent_id")
		}

		cases := []struct {
			label string
			evt   *events.Event
			want  string
		}{
			{"session_start", findByName("session_start"),
				"Session ID: demo-9f2a1c\nMessage ID: msg_0001\nFlow ID: default\n"},
			{"flow_step_start(routing) — Non-Aggregator Count label deliberately renamed off the retired identifier",
				findByStep("flow_step_start", "routing"),
				"Step: routing\nEnabled: true\nAgent Count: 2\nNon-Aggregator Count: 2\n"},
			{"flow_step_end(routing)", findByStep("flow_step_end", "routing"),
				"Step: routing\nEnabled: true\nDuration: 70ms\nRouting Mode: broadcast\nRoute Taken: all\nRoute Agents: " + strings.Join(routingNode.RouteAgents, ", ") + "\n"},
			{"agent_stream_start(first worker)", findBySourceID("agent_stream_start", firstWorker),
				"Agent: " + firstWorker + "\nMessage ID: msg_0004\n"},
			{"agent_stream_start(second worker)", findBySourceID("agent_stream_start", secondWorker),
				"Agent: " + secondWorker + "\nMessage ID: msg_0005\n"},
			{"error", findByName("error"),
				"Error Type: rate_limit_error\nMessage: upstream rate limit hit, retrying\nAgent: " + errorAgent + "\n"},
			{"agent_stream_complete(first worker)", findBySourceID("agent_stream_complete", firstWorker),
				"Agent: " + firstWorker + "\nToken Count: 42\n\nFull Response:\nLet's think about this together. There are a few threads worth pulling on. Taken together, this points toward a clear next step."},
			{"agent_stream_complete(second worker)", findBySourceID("agent_stream_complete", secondWorker),
				"Agent: " + secondWorker + "\nToken Count: 39\n\nFull Response:\nHere's another angle worth considering. It helps to slow down and notice what's present. Continuing from where we left off, the same idea still holds."},
			{"flow_step_start(synthesis)", findByStep("flow_step_start", "synthesis"),
				"Step: synthesis\nEnabled: true\nAgent Count: 2\n"},
			{"flow_step_detail", findByName("flow_step_detail"),
				"Step: synthesis\nPlan ID: plan_001\n\nSynthesis Preview:\nCombining both perspectives into one balanced answer...\n\nPerspectives:\n• " + firstWorker + ": Practical, action-oriented framing.\n• " + secondWorker + ": Grounded, present-moment framing.\n"},
			{"aggregator stream_start", findByKind(events.KindStreamStart),
				"Message ID: msg_0017\n"},
			{"aggregator stream_complete — Aggregator Response label deliberately renamed off the retired identifier",
				findByKind(events.KindStreamEnd),
				"Aggregator Response:\nBringing both viewpoints together, the synthesis suggests a balanced, grounded next step.\n\nTokens: 28\n"},
			{"flow_step_end(synthesis)", findByStep("flow_step_end", "synthesis"),
				"Step: synthesis\nEnabled: true\nDuration: 560ms\n"},
			{"session_complete", findByName("session_complete"),
				"Session ID: demo-9f2a1c\n"},
		}

		for _, tc := range cases {
			if tc.evt == nil {
				t.Errorf("%s: expected the fixture to contain this event", tc.label)
				continue
			}
			got := m.renderEventExpandedPlainText(tc.evt, &msg)
			if got != tc.want {
				t.Errorf("%s: plain-text export changed from baseline:\n got:  %q\n want: %q", tc.label, got, tc.want)
			}
		}
	})

	t.Run("flow_and_timeline_panes_lines_match_baseline", func(t *testing.T) {
		// Neither pane's *collapsed* row rendering (what this fixture
		// exercises — flowExpanded stays empty) has hardcoded
		// aggregator-specific label text (Timeline lists agent IDs
		// verbatim; Flow's per-stage rows use only generic field
		// names), so — unlike Events/App below — these two panes'
		// baseline had no deliberate renames at all here. Flow's
		// *expanded* details view (renderFlowNodeDetails, not
		// exercised by this subtest) does have its own rename
		// (the old role-specific metrics label became "Aggregator Metrics"), covered separately
		// by render_flow_test.go's TestRenderFlowNodeDetails.
		// Compared line-by-line with trailing whitespace trimmed (the
		// %-12s step-name padding is incidental column alignment, not
		// load-bearing content) rather than one exact multi-line
		// literal, so this test doesn't become a trap for invisible
		// trailing-space drift.
		wantFlowLines := []string{
			"Flow Steps",
			"─────────────────────────────────────────",
			"",
			"Turn 1 of 1  ([ ] to navigate)",
			"",
			"> ✓ routing      [70ms] → all [" + strings.Join(routingNode.RouteAgents, ", ") + "]",
			"discovery",
			"agent_exec",
			"filter",
			"✓ synthesis    [560ms]",
			aggregatorStageName,
		}
		assertTrimmedLinesMatch(t, "renderFlowPane", m.renderFlowPane(), wantFlowLines)

		wantTimelineLines := []string{
			"Timeline",
			"─────────────────────────────────────────",
			"",
			"Stages",
			"routing      ████ 70ms",
			"synthesis    ███████████████████████████████████ 560ms",
			"",
			"Total: 630ms",
			"",
			"Agents",
			"✓ " + timelineWorkers[0] + "   ▓▓▓ 3 tokens",
			"✓ " + timelineWorkers[1] + "      ▓▓▓ 3 tokens",
			"✓ " + aggregatorStageName + "          ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓ 28 tokens",
		}
		assertTrimmedLinesMatch(t, "renderTimelinePane", m.renderTimelinePane(), wantTimelineLines)
	})

	t.Run("events_and_app_panes_load_bearing_content_unchanged", func(t *testing.T) {
		// These two panes DO have aggregator-labeling text this
		// task deliberately renames to "Aggregator" (see
		// staleAggregatorLabel), so a byte-for-byte baseline diff would
		// incorrectly fail on the very rename this task requires. Assert
		// the load-bearing data (numeric values, agent content, event
		// lines) survives unchanged, and that the label rename actually
		// landed.
		eventsPaneText := stripANSI(m.renderEventsPane())
		for _, want := range []string{
			"Events (View: PARSED) (Tokens: OFF) (Aggregator: OFF)",
			"Ctrl+T: RAW view, t: tokens, W: aggregator-only",
			"🧙 Aggregator: 28 tokens ✓",
			"[flow_step_start] step=routing enabled=true",
			"[flow_step_end] step=routing duration=70ms",
			"[agent_stream_start] agent=" + firstWorker,
			"[agent_stream_complete] agent=" + firstWorker + " tokens=42",
			"[agent_stream_complete] agent=" + secondWorker + " tokens=39",
			"[flow_step_end] step=synthesis duration=560ms",
		} {
			if !strings.Contains(eventsPaneText, want) {
				t.Errorf("renderEventsPane: expected to contain %q, got:\n%s", want, eventsPaneText)
			}
		}

		app := stripANSI(m.renderAppPane())
		for _, want := range []string{
			"Aggregator Output",
			"[Aggregator] tokens: 28, ✓",
			"Bringing both viewpoints together, the synthesis suggests a balanced, grounded next step.",
			secondWorker + " (3 tokens)",
			"Here's another angle worth considering.",
			firstWorker + " (3 tokens)",
			"Let's think about this together.",
		} {
			if !strings.Contains(app, want) {
				t.Errorf("renderAppPane: expected to contain %q, got:\n%s", want, app)
			}
		}
	})
}

// assertTrimmedLinesMatch splits got into lines, trims each with
// strings.TrimSpace (so incidental column-padding differences aren't
// load-bearing — see the comment on the flow/timeline pane subtest
// above for why), and asserts the result equals want exactly, line for
// line.
func assertTrimmedLinesMatch(t *testing.T, label, got string, want []string) {
	t.Helper()
	// Strip SGR so parity holds whether lipgloss is forced truecolor (VHS/demo)
	// or monochrome under NO_COLOR.
	got = stripANSI(got)
	gotLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	for i := range gotLines {
		gotLines[i] = strings.TrimSpace(gotLines[i])
	}
	if len(gotLines) != len(want) {
		t.Errorf("%s: expected %d lines, got %d\n got:  %#v\nwant: %#v", label, len(want), len(gotLines), gotLines, want)
		return
	}
	for i := range want {
		if gotLines[i] != want[i] {
			t.Errorf("%s: line %d changed from baseline:\n got:  %q\nwant: %q", label, i, gotLines[i], want[i])
		}
	}
}
