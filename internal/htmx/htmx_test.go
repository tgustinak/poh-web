package htmx

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"
)

func decodeTrigger(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()

	raw := rec.Header().Get("HX-Trigger")
	for _, r := range raw {
		if r > unicode.MaxASCII {
			t.Fatalf("expected ASCII-only HX-Trigger, got %q", raw)
		}
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode HX-Trigger: %v", err)
	}

	return payload
}

func TestTriggerEncodesEvents(t *testing.T) {
	rec := httptest.NewRecorder()

	Trigger(rec, Events{ShowToast: "Koncert pridaný.", FormReset: "concert-create-form"})

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	payload := decodeTrigger(t, rec)
	if payload["showToast"] != "Koncert pridaný." {
		t.Fatalf("expected toast message, got %q", payload)
	}

	if payload["formReset"] != "concert-create-form" {
		t.Fatalf("expected form reset target, got %q", payload)
	}
}

func TestTriggerOmitsEmptyEvents(t *testing.T) {
	rec := httptest.NewRecorder()

	Trigger(rec, Events{})

	if payload := decodeTrigger(t, rec); len(payload) != 0 {
		t.Fatalf("expected empty payload, got %q", payload)
	}
}

func TestToast(t *testing.T) {
	rec := httptest.NewRecorder()

	Toast(rec, "Ahoj")

	payload := decodeTrigger(t, rec)
	if payload["showToast"] != "Ahoj" {
		t.Fatalf("expected toast message, got %q", payload)
	}

	if _, ok := payload["formReset"]; ok {
		t.Fatalf("expected no formReset, got %q", payload)
	}

	if !strings.Contains(rec.Header().Get("HX-Trigger"), "showToast") {
		t.Fatal("expected showToast event")
	}
}
