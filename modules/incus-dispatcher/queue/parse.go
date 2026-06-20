package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ParseDirective decodes a single JSON directive payload with STRICT field
// checking: any unknown field (e.g. "access_cmd", "root") causes an error,
// enforcing D1 — directives carry only intent + proposed template, never
// direct commands or privilege flags.
//
// This is the strict ingestion BOUNDARY for directives that arrive as JSON over
// the wire. The current in-process MemoryQueue is fed typed Directive structs, so
// no caller deserializes JSON yet; ParseDirective is wired in when the real queue
// substrate (laneq) lands and directives cross a JSON boundary — see ITER-0006.
// Until then it is the unit-seam evidence for STORY-0049 AC-1 (SCENARIO-0026):
// the strict decoder is delivered and proven now; its activation rides the substrate.
func ParseDirective(data []byte) (Directive, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var d Directive
	if err := dec.Decode(&d); err != nil {
		return Directive{}, err
	}

	// Reject trailing content after the JSON object.
	if dec.More() {
		return Directive{}, fmt.Errorf("parse: trailing data after directive object")
	}

	return d, nil
}
