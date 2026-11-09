package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// MaxPropertiesLength is the Event pipeline's limit on properties, as JSON.
const MaxPropertiesLength = 2048

var (
	eventNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
	propertyKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// ErrInvalid marks an event that breaks the standard. The client drops it.
var ErrInvalid = errors.New("invalid event")

// ValidateEventName checks the noun.past_verb pattern, e.g. "recording.started".
func ValidateEventName(name string) error {
	if !eventNamePattern.MatchString(name) {
		return fmt.Errorf("%w: event name %q is not noun.past_verb", ErrInvalid, name)
	}
	return nil
}

// ValidateProperties checks snake_case top-level keys and the 2,048-byte
// limit on the compact JSON encoding.
func ValidateProperties(props map[string]any) error {
	for key := range props {
		if !propertyKeyPattern.MatchString(key) {
			return fmt.Errorf("%w: property key %q is not snake_case", ErrInvalid, key)
		}
	}
	if len(props) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(props); err != nil {
		return fmt.Errorf("%w: properties are not JSON: %v", ErrInvalid, err)
	}
	if n := buf.Len() - 1; n > MaxPropertiesLength { // Encode adds a newline
		return fmt.Errorf("%w: properties are %d bytes as JSON, over %d", ErrInvalid, n, MaxPropertiesLength)
	}
	return nil
}
