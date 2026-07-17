# Ship-Point Checklist — Phases 001–005

Verified 2026-07-15, at the close of `005_DIALECT_TOOLING/01_init_and_explain`
(the design's "ship point": init + explain, dialects-as-data acid test
passed, the goal is essentially met). Source: `FESTIVAL_GOAL.md` Functional
Success, items 1–4.

## 1. `git clone && just demo` shows a real multi-agent timeline with no backend, key, or network

**Verified.** Cloned the branch into a scratch directory and ran `just demo`
cold — it builds the binary and runs `timeline testdata/fixtures/
brainyard-session.jsonl`, rendering a full parallel-execution timeline
(sam_harris / eckhart_tolle streaming concurrently) with zero network calls,
zero API keys, zero backend.

## 2. `dialects/brainyard.yaml` expresses all 19 event types, the session handshake, and the wizard pseudo-agent in pure YAML — zero Go (the acid test)

**Verified.**
- All 19 Brainyard event types present as distinct `match: {event: ...}`
  targets across 14 rules (one rule covers 6 detail-kind events as a single
  `event: [...]` list): `session_start`, `session_complete`,
  `agent_stream_start`, `agent_content`, `agent_stream_complete`,
  `wizard_stream_start`, `wizard_content`, `wizard_stream_complete`,
  `flow_step_start`, `flow_step_end`, `flow_config`, `agent_metadata`,
  `error`, `prompt_info`, `prompt_full`, `filter_detail`,
  `perspective_detail`, `synthesis_detail`, `flow_step_detail`.
- `setup:` (session handshake) and `send:` (turn initiation) both declared.
- The wizard pseudo-agent — no `agent_id` on the wire — is exactly
  `source: {const: wizard}`, three times.
- `ls internal/dialects/*.go` → no such thing. `internal/mapping/` (the
  engine) never mentions Brainyard, wizard, or any other system by name.

## 3. Three shipped dialects covering three wire shapes: named events (brainyard), no names (openai, structural matching), name-in-payload (anthropic)

**Verified.**
| Dialect | `discriminator:` |
|---|---|
| `dialects/brainyard.yaml` | `event` |
| `dialects/openai.yaml` | `auto` |
| `dialects/anthropic.yaml` | `{path: type}` |

Each has a hand-authored fixture and an exact per-frame decode test
(`dialects/dialects_test.go`), plus a golden `explain` trace
(`testdata/goldens/*.explain.txt`, this task).

## 4. `init` infers a usable draft dialect from a live stream or recording; `explain` shows per-frame rule matching

**Verified.**
- `stream-debugger init --from <fixture>` emits a commented draft YAML;
  recovers 100% of hand-written `brainyard.yaml`'s event coverage and
  source/content/seq paths (required: ≥70%), and correctly infers
  `discriminator: auto` for the openai fixture (no event names, no payload
  discriminator field).
- `stream-debugger explain --dialect d.yaml --from fixture.jsonl` traces
  every frame: matched rule + extracted fields, or a hint when nothing
  matched. Exit code reflects health (0 clean, 1 on unknowns/empty
  extractions) — verified against the real `brainyard.yaml` (22/22 frames,
  exit 0) and against a deliberately broken dialect (wrong `content:` path,
  exit 1, `⚠ empty` pointing at the exact rule and path).
- All three dialects now have a golden `explain` trace under CI
  (`testdata/goldens/`, `just goldens` to regenerate); a deliberate
  regression (renaming `agent_content`) breaks the golden match, proven by
  `TestExplainGoldens_DetectsRenamedEventRegression`.

## Also true, beyond the four items above

- Unknown events are never dropped: `TestEngine_Decode_NeverErrorsOnUnknown`,
  `TestParser_NeverErrorsOnUnknownEventType`,
  `TestParser_NeverErrorsOnMalformedJSON` all green.
- The mapping engine and visualizer now keep dialect-specific vocabulary in
  data and dispatch through normalized events and flow roles.
- `r3labs/sse` and the 2019 `golang.org/x/net` pseudo-version pin it dragged
  in are gone from `go.mod`.

## Not yet true (later phases)

- gRPC transport (006), A2A dialect (008), and everything in 009_PUBLISH
  (explicitly human-gated).
