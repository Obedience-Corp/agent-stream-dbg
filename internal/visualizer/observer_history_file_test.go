package visualizer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	"io"
	"os"
	"strings"
	"testing"
)

func TestHistoryExactFramingAndRecovery(t *testing.T) {
	valid := historyTurn("old", 10)
	prefix := historyHeader + "malformed\r\n" + strings.Repeat("x", filetransport.MaxLineBytes+1) + "\n"
	request := historyFixture(t, prefix+valid+`{"type":"delta","turn_id":"partial"}`)
	var offsets []int64
	var raw [][]byte
	err := historyScan(context.Background(), request, 0, func(_ *events.Event, f transport.Frame, _ string) bool {
		offsets = append(offsets, f.Offset)
		raw = append(raw, bytes.Clone(f.Raw))
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 3 || offsets[2] != int64(len(prefix)) || string(raw[2]) != valid {
		t.Fatalf("framing drift: offsets=%v raw=%q", offsets, raw)
	}
}
func TestHistoryHeaderBoundAndPartial(t *testing.T) {
	for _, body := range []string{strings.Repeat("x", filetransport.MaxLineBytes*3) + "\n", `{"type":"header"}`} {
		request := historyFixture(t, body)
		if f, err := historyOpen(context.Background(), request); err == nil {
			_ = f.Close()
			t.Fatal("accepted oversized/partial header")
		}
	}
	reader := bufio.NewReaderSize(strings.NewReader(strings.Repeat("x", filetransport.MaxLineBytes*3)+"\nnext\r\n"), 4096)
	raw, size, err := historyLine(context.Background(), reader)
	if !errors.Is(err, errHistoryOversize) || raw != nil || size != 3*filetransport.MaxLineBytes+1 {
		t.Fatal("oversize not bounded/drained", size, err)
	}
	raw, _, err = historyLine(context.Background(), reader)
	if err != nil || string(raw) != "next\r\n" {
		t.Fatal("failed resync", err)
	}
	_, _, err = historyLine(context.Background(), reader)
	if !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}
func TestHistoryDetectsSameInodeMutationDuringScan(t *testing.T) {
	body := historyHeader + historyTurn("one", 10)
	request := historyFixture(t, body)
	mutated := false
	err := historyScan(context.Background(), request, 0, func(_ *events.Event, _ transport.Frame, _ string) bool {
		if !mutated {
			mutated = true
			replacement := strings.Replace(body, "capture", "changed", 1)
			if err := os.WriteFile(request.Path, []byte(replacement), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return true
	})
	if !errors.Is(err, filetransport.ErrSourceChanged) {
		t.Fatal("mixed source accepted", err)
	}
}

func TestHistoryCancelledHeaderStopsBeforeRead(t *testing.T) {
	request := historyFixture(t, strings.Repeat("x", filetransport.MaxLineBytes*3))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if file, err := historyOpen(ctx, request); !errors.Is(err, context.Canceled) {
		if file != nil {
			_ = file.Close()
		}
		t.Fatal("cancelled header read continued", err)
	}
}
