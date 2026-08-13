package sse

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEncode_SimpleString(t *testing.T) {
	var buf bytes.Buffer
	event := Event{
		Event: "message",
		Data:  "some data",
	}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expected := "event: message\ndata: some data\n\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEncode_MultiLineData(t *testing.T) {
	var buf bytes.Buffer
	event := Event{
		Event: "message",
		Data:  "some data\nmore data",
	}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expected := "event: message\ndata: some data\ndata: more data\n\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEncode_WithID(t *testing.T) {
	var buf bytes.Buffer
	event := Event{
		ID:    "124",
		Event: "message",
		Data:  "some data",
	}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expected := "id: 124\nevent: message\ndata: some data\n\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEncode_WithRetry(t *testing.T) {
	var buf bytes.Buffer
	event := Event{
		Event: "message",
		Data:  "some data",
		Retry: 3000,
	}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expected := "event: message\nretry: 3000\ndata: some data\n\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEncode_CommentOnly(t *testing.T) {
	var buf bytes.Buffer
	event := Event{Comment: "ping"}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expected := ": ping\n\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEncode_MultiLineComment(t *testing.T) {
	var buf bytes.Buffer
	event := Event{Comment: "first\nsecond"}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expected := ": first\n: second\n\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEncode_CommentPrecedesFields(t *testing.T) {
	var buf bytes.Buffer
	event := Event{Comment: "note", ID: "7", Event: "message", Data: "payload"}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expected := ": note\nid: 7\nevent: message\ndata: payload\n\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEncode_EmptyStringData(t *testing.T) {
	var buf bytes.Buffer
	event := Event{Event: "message", Data: ""}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// An empty payload still writes a data line, so the client dispatches an
	// event whose payload is the empty string.
	expected := "event: message\ndata: \n\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEncode_ComplexData(t *testing.T) {
	var buf bytes.Buffer
	event := Event{
		ID:    "124",
		Event: "message",
		Data: map[string]any{
			"user":    "manu",
			"date":    1431540810,
			"content": "hi!",
		},
	}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Parse the JSON to verify it's valid
	output := buf.String()
	lines := bytes.Split(bytes.TrimSpace([]byte(output)), []byte("\n"))

	var dataLine string
	for _, line := range lines {
		if bytes.HasPrefix(line, []byte("data: ")) {
			dataLine = string(line[6:]) // Remove "data: " prefix
			break
		}
	}

	if dataLine == "" {
		t.Fatal("No data line found")
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(dataLine), &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if result["user"] != "manu" {
		t.Errorf("Expected user to be 'manu', got %v", result["user"])
	}
}

func TestEncode_EmptyObject(t *testing.T) {
	var buf bytes.Buffer
	event := Event{
		ID:    "123",
		Event: "message",
		Data:  map[string]any{}, // Empty object must still be sent as data: {}
	}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Should include data: {} even for empty object
	output := buf.String()
	if !strings.Contains(output, "data: {}") {
		t.Errorf("Expected output to contain 'data: {}', got %q", output)
	}
}

func TestEncode_NilData(t *testing.T) {
	var buf bytes.Buffer
	event := Event{
		ID:    "123",
		Event: "message",
		Data:  nil, // No data field should be written
	}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expected := "id: 123\nevent: message\n\n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestEncode_FieldOrder(t *testing.T) {
	var buf bytes.Buffer
	event := Event{
		ID:    "124",
		Event: "message",
		Retry: 3000,
		Data:  "test",
	}

	if err := Encode(&buf, event); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	output := buf.String()
	// Check that fields are in recommended order: id, event, retry, data
	idPos := strings.Index(output, "id: ")
	eventPos := strings.Index(output, "event: ")
	retryPos := strings.Index(output, "retry: ")
	dataPos := strings.Index(output, "data: ")

	if idPos == -1 || eventPos == -1 || retryPos == -1 || dataPos == -1 {
		t.Fatalf("Missing required fields in output: %q", output)
	}

	if idPos >= eventPos || eventPos >= retryPos || retryPos >= dataPos {
		t.Errorf("Fields not in recommended order. Output: %q", output)
	}
}

func TestEncode_PrimitiveTypes(t *testing.T) {
	tests := []struct {
		name     string
		data     any
		expected string
	}{
		{"int", 42, "data: 42\n\n"},
		{"int64", int64(42), "data: 42\n\n"},
		{"float64", 3.14, "data: 3.14\n\n"},
		{"bool", true, "data: true\n\n"},
		{"bool false", false, "data: false\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			event := Event{
				Data: tt.data,
			}

			if err := Encode(&buf, event); err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			if !bytes.Contains(buf.Bytes(), []byte(tt.expected[:len(tt.expected)-1])) {
				t.Errorf("Expected output to contain %q, got %q", tt.expected, buf.String())
			}
		})
	}
}

func TestEncode_RejectsInvalidEvents(t *testing.T) {
	tests := []struct {
		name  string
		event Event
	}{
		{"empty event", Event{}},
		{"id with newline", Event{ID: "1\n2", Data: "x"}},
		{"id with carriage return", Event{ID: "1\r2", Data: "x"}},
		{"id with NUL", Event{ID: "1\x002", Data: "x"}},
		{"event type with newline", Event{Event: "a\nb", Data: "x"}},
		{"event type with carriage return", Event{Event: "a\rb", Data: "x"}},
		{"comment with carriage return", Event{Comment: "a\rb"}},
		{"negative retry", Event{Retry: -1, Data: "x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Encode(&buf, tt.event); err == nil {
				t.Fatalf("Expected an error, got output %q", buf.String())
			}
			if buf.Len() != 0 {
				t.Errorf("Expected no bytes written for a rejected event, got %q", buf.String())
			}
		})
	}
}
