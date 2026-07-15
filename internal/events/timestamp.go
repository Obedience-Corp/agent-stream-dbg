package events

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// ParseTimestamp tolerantly decodes a wire timestamp from any realistic
// format: RFC3339(+nano) strings, epoch seconds, epoch millis, or the
// protobuf Timestamp JSON form ({"seconds":N,"nanos":N}). Absent or
// unrecognized input returns the local receipt time — never an error, since
// a malformed timestamp is not a reason to drop an otherwise-decodable event.
func ParseTimestamp(raw json.RawMessage) time.Time {
	receiptTime := time.Now()

	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return receiptTime
	}

	// Quoted string: RFC3339(+nano), or a numeric string.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return receiptTime
		}
		if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
			return t
		}
		if n, err := strconv.ParseFloat(str, 64); err == nil {
			if t, ok := timeFromEpochNumber(n); ok {
				return t
			}
		}
		return receiptTime
	}

	// protobuf Timestamp JSON form: {"seconds": N, "nanos": N}
	if len(s) > 0 && s[0] == '{' {
		var pb struct {
			Seconds int64 `json:"seconds"`
			Nanos   int32 `json:"nanos"`
		}
		if err := json.Unmarshal(raw, &pb); err == nil && (pb.Seconds != 0 || pb.Nanos != 0) {
			return time.Unix(pb.Seconds, int64(pb.Nanos)).UTC()
		}
		return receiptTime
	}

	// Bare number: epoch seconds or millis.
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		if t, ok := timeFromEpochNumber(n); ok {
			return t
		}
	}

	return receiptTime
}

// timeFromEpochNumber applies the seconds-vs-millis heuristic: values above
// 1e12 are treated as milliseconds since epoch, values above 1e9 as seconds.
// Smaller values aren't recognizable epoch times, so ok is false.
func timeFromEpochNumber(n float64) (time.Time, bool) {
	switch {
	case n > 1e12:
		return time.UnixMilli(int64(n)).UTC(), true
	case n > 1e9:
		return time.Unix(int64(n), 0).UTC(), true
	default:
		return time.Time{}, false
	}
}
