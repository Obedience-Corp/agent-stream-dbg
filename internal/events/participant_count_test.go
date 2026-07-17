package events

import "testing"

func TestEventParticipantCount(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]any
		want   int
	}{
		{name: "normalized", fields: map[string]any{"participant_count": 3}, want: 3},
		{name: "dialect field", fields: map[string]any{"non_primary_count": float64(2)}, want: 2},
		{name: "agent count is not participant count", fields: map[string]any{"agent_count": 4}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (&Event{Fields: tt.fields}).ParticipantCount()
			if got != tt.want {
				t.Errorf("ParticipantCount() = %d, want %d", got, tt.want)
			}
		})
	}
}
