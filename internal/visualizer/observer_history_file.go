package visualizer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	"io"
	"os"
	"strings"
)

func historyOpen(ctx context.Context, request historyRequest) (*os.File, error) {
	f, err := filetransport.OpenRegular(request.Path)
	if err != nil {
		return nil, fmt.Errorf("open source history: %w", errors.Join(filetransport.ErrSourceChanged, err))
	}
	fail := func(err error) (*os.File, error) { _ = f.Close(); return nil, err }
	info, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	minimum := request.MinSize
	if request.Identity != nil {
		minimum = max(minimum, request.Identity.Size())
		if !os.SameFile(request.Identity, info) {
			return fail(filetransport.ErrSourceChanged)
		}
	}
	if !info.Mode().IsRegular() || info.Size() < minimum {
		return fail(filetransport.ErrSourceChanged)
	}
	if len(request.Prefix) > 0 {
		prefix := make([]byte, len(request.Prefix))
		if _, err := f.ReadAt(prefix, 0); err != nil || !bytes.Equal(prefix, request.Prefix) {
			return fail(filetransport.ErrSourceChanged)
		}
	}
	reader := bufio.NewReaderSize(f, 64<<10)
	header, _, err := historyLine(ctx, reader, true)
	if err != nil {
		return fail(fmt.Errorf("source header unavailable: %w", err))
	}
	if len(header) > filetransport.MaxLineBytes {
		return fail(fmt.Errorf("source header exceeds bounded line size"))
	}
	event, err := request.Parser.ParseBounded("", header, 4<<20)
	if err != nil {
		return fail(err)
	}
	for _, check := range [][2]string{{"capture_id", request.CaptureID}, {"trace_schema", request.Schema}, {"runtime_id", request.Runtime}, {"session_id", request.Session}, {"conversation_epoch", request.Epoch}} {
		if event.StringField(check[0]) != check[1] {
			return fail(filetransport.ErrSourceChanged)
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return f, nil
}
func historyScan(ctx context.Context, request historyRequest, start int64, visit func(*events.Event, transport.Frame, string) bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := historyOpen(ctx, request)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(f, 64<<10)
	offset := start
	state := newObserverStore()
	state.legacy = request.Legacy
	for {
		raw, size, readErr := historyLine(ctx, reader)
		lineOffset := offset
		offset += size
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			if errors.Is(readErr, errHistoryOversize) {
				continue
			}
			return readErr
		}
		event, parseErr := request.Parser.ParseBounded("", bytes.TrimSpace(raw), 4<<20)
		if parseErr != nil {
			continue
		}
		if capture := event.StringField("capture_id"); capture != "" && request.CaptureID != "" && capture != request.CaptureID {
			return filetransport.ErrSourceChanged
		}
		scope := state.scopeFor(event)
		frame := transport.Frame{Data: bytes.TrimSpace(raw), Raw: raw, Offset: lineOffset, Size: size}
		if !visit(event, frame, scope) {
			break
		}
	}
	current, err := os.Stat(request.Path)
	if err != nil {
		return filetransport.ErrSourceChanged
	}
	info, err := f.Stat()
	if err != nil || !os.SameFile(info, current) || current.Size() < request.MinSize {
		return filetransport.ErrSourceChanged
	}
	if request.Identity != nil && !os.SameFile(request.Identity, current) {
		return filetransport.ErrSourceChanged
	}
	// Reopen/revalidate after reading too: a same-inode header replacement
	// while the visitor was running must not publish mixed evidence.
	verified, verifyErr := historyOpen(ctx, request)
	if verifyErr != nil {
		return verifyErr
	}
	_ = verified.Close()
	return ctx.Err()
}
func loadObserverHistory(ctx context.Context, request historyRequest) (*observerStore, observerSelection, error) {
	target := request.Anchor
	payloadStart := int64(0)
	if request.Direction != 0 {
		// Sparse windows may contain delayed observations of much older turns.
		// Scan the complete source to establish the nearest source-clock neighbor;
		// the index is only a seek hint, never proof of predecessor coverage.
		var chosen *observedRecord
		// Only equal-clock comparisons need first-occurrence lookup. Cache a
		// bounded subset; cache exhaustion falls back to a cancellable prefix
		// scan rather than imposing a maximum number of turns in the artifact.
		firstOffsets := make(map[string]int64)
		var discoveryErr error
		err := historyScan(ctx, request, 0, func(event *events.Event, frame transport.Frame, scope string) bool {
			if event.StringField("turn_id") == "" && !strings.HasPrefix(scope, "legacy-local/") {
				return true
			}
			if scope == request.Anchor.Key {
				return true
			}
			if chosen != nil && chosen.Scope == scope {
				return true
			}
			first := frame.Offset
			value, known := finiteNumber(event.Fields["turn_start_ms"])
			tied := !known || request.Anchor.StartMS == nil || value == *request.Anchor.StartMS
			if chosen != nil {
				other, ok := finiteNumber(chosen.Event.Fields["turn_start_ms"])
				tied = tied || known && ok && value == other
			}
			if tied {
				cached, ok := firstOffsets[scope]
				if !ok {
					lookupErr := historyScan(ctx, request, 0, func(_ *events.Event, frame transport.Frame, candidate string) bool {
						if candidate == scope {
							cached = frame.Offset
							return false
						}
						return true
					})
					if lookupErr != nil {
						discoveryErr = lookupErr
						return false
					}
					if len(firstOffsets) < 128 {
						firstOffsets[scope] = cached
					}
				}
				first = cached
			}
			record := &observedRecord{Event: event, Offset: first, Scope: scope}
			if neighborRecord(request.Anchor, record, request.Direction) && betterNeighbor(chosen, record, request.Direction) {
				chosen = &observedRecord{Scope: scope, Offset: first, Event: &events.Event{Fields: map[string]any{
					"runtime_id":    strings.Clone(boundedIdentity(event.StringField("runtime_id"))),
					"turn_id":       strings.Clone(boundedIdentity(event.StringField("turn_id"))),
					"turn_start_ms": nil,
				}}}
				if known {
					chosen.Event.Fields["turn_start_ms"] = value
				}
			}
			return true
		})
		if err != nil {
			return nil, observerSelection{}, err
		}
		if discoveryErr != nil {
			return nil, observerSelection{}, discoveryErr
		}
		found := chosen != nil
		if found {
			target = observerSelection{Key: chosen.Scope, Offset: chosen.Offset}
			if !request.Legacy {
				for _, entry := range request.Index {
					if entry.Offset <= chosen.Offset && entry.Offset > payloadStart {
						payloadStart = entry.Offset
					}
				}
			}
		}
		if !found {
			return nil, observerSelection{}, nil
		}
	}
	// A filtered scan from a proven sparse predecessor preserves every delayed
	// observation for this immutable scope; the index never defines turn order. A long single turn still obeys the same page cap.
	page := newObserverStore()
	page.legacy = request.Legacy
	err := historyScan(ctx, request, payloadStart, func(event *events.Event, frame transport.Frame, scope string) bool {
		if scope == target.Key {
			page.add(event, frame, false, scope)
			if request.OwnBytes != nil {
				request.OwnBytes(page.bytes)
			}
		}
		return true
	})
	if err != nil {
		return nil, observerSelection{}, err
	}
	anchor, found := selectionFromRecords(page, target.Key)
	if found {
		anchor.Offset = min(anchor.Offset, target.Offset)
	}
	if !found {
		return nil, observerSelection{}, nil
	}
	return page, anchor, nil
}

var errHistoryOversize = errors.New("history record exceeds bounded line size")

// historyLine counts exact bytes, retains CRLF, and never interprets an
// unterminated tail as a complete record. Oversized rows are drained to newline.
func historyLine(ctx context.Context, reader *bufio.Reader, rejectOversize ...bool) ([]byte, int64, error) {
	var raw []byte
	var size int64
	oversized := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, size, err
		}
		fragment, err := reader.ReadSlice('\n')
		size += int64(len(fragment))
		if !oversized {
			if len(raw)+len(fragment) > filetransport.MaxLineBytes {
				oversized = true
				raw = nil
				if len(rejectOversize) > 0 && rejectOversize[0] {
					return nil, size, errHistoryOversize
				}
			} else {
				raw = append(raw, fragment...)
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil {
			return raw, size, err
		}
		if oversized {
			return nil, size, errHistoryOversize
		}
		return raw, size, nil
	}
}

// Immediate user selections supersede any queued asynchronous page result.
