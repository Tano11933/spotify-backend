package logging

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

// TestSetupToJSONAndLogBridge mengunci dua hal: format JSON dipakai saat
// diminta, dan panggilan log.Printf lama ikut dialirkan ke slog lewat bridge
// supaya outputnya tidak terbelah dua format.
func TestSetupToJSONAndLogBridge(t *testing.T) {
	var buffer bytes.Buffer
	SetupTo(&buffer, "json")

	slog.Info("structured", "key", "value")
	log.Printf("legacy line")

	output := buffer.String()

	if !strings.Contains(output, `"msg":"structured"`) {
		t.Fatalf("log slog tidak berformat JSON: %s", output)
	}
	if !strings.Contains(output, `"key":"value"`) {
		t.Fatalf("atribut slog hilang: %s", output)
	}
	if !strings.Contains(output, `"msg":"legacy line"`) {
		t.Fatalf("log.Printf tidak diteruskan ke slog: %s", output)
	}
	if !strings.Contains(output, `"level":"INFO"`) {
		t.Fatalf("level tidak tercatat: %s", output)
	}
}

// TestSetupToTextFormat memastikan default non-JSON memakai handler text yang
// enak dibaca di terminal.
func TestSetupToTextFormat(t *testing.T) {
	var buffer bytes.Buffer
	SetupTo(&buffer, "text")

	slog.Warn("plain warning")

	output := buffer.String()
	if !strings.Contains(output, "level=WARN") || !strings.Contains(output, "msg=\"plain warning\"") {
		t.Fatalf("format text tidak sesuai: %s", output)
	}
}
