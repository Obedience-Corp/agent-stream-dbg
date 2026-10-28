package events

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

// ParseTimestamp tolerantly decodes a wire timestamp from any realistic
// format: RFC3339(+nano) strings, epoch seconds, epoch millis, or the
// {"seconds":N,"nanos":N} epoch JSON shape. Absent or unrecognized input
// returns the local receipt time — never an error, since a malformed
// timestamp is not a reason to drop an otherwise-decodable event.
func ParseTimestamp(raw json.RawMessage) time.Time {
	if at, ok := ParseSourceTimestamp(raw); ok {
		return at
	}
	return time.Now()
}

// ParseSourceTimestamp distinguishes a measured source wall timestamp from the
// receipt fallback retained by ordinary interactive decoders.
func ParseSourceTimestamp(raw json.RawMessage) (time.Time, bool) {

	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return time.Time{}, false
	}

	// Quoted string: RFC3339(+nano), or a numeric string.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return time.Time{}, false
		}
		if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
			return t, true
		}
		if n, err := strconv.ParseFloat(str, 64); err == nil {
			if t, ok := timeFromEpochNumber(n); ok {
				return t, true
			}
		}
		return time.Time{}, false
	}

	// The {"seconds": N, "nanos": N} epoch JSON shape.
	if len(s) > 0 && s[0] == '{' {
		var pb struct {
			Seconds int64 `json:"seconds"`
			Nanos   int32 `json:"nanos"`
		}
		if err := json.Unmarshal(raw, &pb); err == nil && (pb.Seconds != 0 || pb.Nanos != 0) {
			return time.Unix(pb.Seconds, int64(pb.Nanos)).UTC(), true
		}
		return time.Time{}, false
	}

	// Bare number: epoch seconds or millis.
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		if t, ok := timeFromEpochNumber(n); ok {
			return t, true
		}
	}

	return time.Time{}, false
}

// timeFromEpochNumber applies the seconds-vs-millis heuristic: values above
// 1e12 are treated as milliseconds since epoch, values above 1e9 as seconds.
// Smaller values aren't recognizable epoch times, so ok is false.
func timeFromEpochNumber(n float64) (time.Time, bool) {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return time.Time{}, false
	}
	switch {
	case n > 1e12:
		return time.UnixMilli(int64(n)).UTC(), true
	case n > 1e9:
		return time.Unix(int64(n), 0).UTC(), true
	default:
		return time.Time{}, false
	}
}
