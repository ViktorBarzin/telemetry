package telemetry

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"testing/quick"
)

var (
	pipelineEvent = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
	pipelineKey   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

func TestValidateEventName(t *testing.T) {
	tests := []struct {
		name string
		ok   bool
	}{
		{"recording.started", true},
		{"trip.created", true},
		{"a1_b.c2_d", true},
		{"", false},
		{"recording", false},
		{"Recording.started", false},
		{"recording.Started", false},
		{"recording.started.now", false},
		{"1recording.started", false},
		{"recording-x.started", false},
		{"recording.started\n", false},
		{" recording.started", false},
		{"recording.", false},
		{".started", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEventName(tt.name)
			if (err == nil) != tt.ok {
				t.Fatalf("ValidateEventName(%q) = %v, want ok=%v", tt.name, err, tt.ok)
			}
		})
	}
}

func TestValidateEventNameAgreesWithThePipeline(t *testing.T) {
	alphabet := []rune("abcz09_.A- \n")
	property := func(picks []uint8) bool {
		var b strings.Builder
		for _, p := range picks {
			b.WriteRune(alphabet[int(p)%len(alphabet)])
		}
		name := b.String()
		return (ValidateEventName(name) == nil) == pipelineEvent.MatchString(name)
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 5000}); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePropertyKeysAgreeWithThePipeline(t *testing.T) {
	property := func(key string) bool {
		err := ValidateProperties(map[string]any{key: 1})
		return (err == nil) == pipelineKey.MatchString(key)
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProperties(t *testing.T) {
	tests := []struct {
		name  string
		props map[string]any
		ok    bool
	}{
		{"nil", nil, true},
		{"snake", map[string]any{"channel_id": 4, "x1": "y"}, true},
		{"nested values", map[string]any{"items": []any{1, "a", map[string]any{"Nested": true}}}, true},
		{"camel", map[string]any{"channelId": 4}, false},
		{"kebab", map[string]any{"a-b": 1}, false},
		{"leading underscore", map[string]any{"_a": 1}, false},
		{"leading digit", map[string]any{"9a": 1}, false},
		{"empty key", map[string]any{"": 1}, false},
		{"exactly 2048", map[string]any{"text": strings.Repeat("a", 2048-len(`{"text":""}`))}, true},
		{"2049", map[string]any{"text": strings.Repeat("a", 2049-len(`{"text":""}`))}, false},
		{"utf8 bytes count", map[string]any{"t": strings.Repeat("é", 1025)}, false},
		{"not json", map[string]any{"ch": make(chan int)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProperties(tt.props)
			if (err == nil) != tt.ok {
				t.Fatalf("ValidateProperties = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestLengthLimitIsCompactJSONBytes(t *testing.T) {
	property := func(n uint16) bool {
		props := map[string]any{"text": strings.Repeat("a", int(n)%4000)}
		raw, _ := json.Marshal(props)
		return (ValidateProperties(props) == nil) == (len(raw) <= MaxPropertiesLength)
	}
	if err := quick.Check(property, nil); err != nil {
		t.Fatal(err)
	}
}
