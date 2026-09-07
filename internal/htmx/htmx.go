// Package htmx provides response helpers for htmx-driven interactions:
// HX-Redirect navigation, HX-Trigger toast notifications and request
// detection. They are UI transport concerns shared by any handler,
// not authentication logic.
package htmx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf16"
)

// IsHtmxRequest reports whether r was made by htmx.
// htmx requests carry an HX-Request header and expect HX-Redirect /
// HX-Trigger driven responses instead of full-page renders.
func IsHtmxRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// Redirect navigates to target: via HX-Redirect for htmx requests,
// via a 303 redirect otherwise (progressive enhancement fallback).
func Redirect(w http.ResponseWriter, r *http.Request, target string) {
	if IsHtmxRequest(r) {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusOK)

		return
	}

	http.Redirect(w, r, target, http.StatusSeeOther)
}

// Toast notifies htmx clients with a toast message via HX-Trigger.
// The admin shell listens for the showToast event (see AdminScripts).
// A 200 status is used deliberately: htmx skips trigger processing
// for error responses.
func Toast(w http.ResponseWriter, msg string) {
	Trigger(w, Events{ShowToast: msg})
}

// Events is the abstract set of client-side events sent via HX-Trigger.
// Field names are enforced by the compiler; each needs a
// document.body listener on the client (see AdminScripts).
// No entity names belong here — payloads carry the specifics
// (e.g. FormReset holds a form element id).
type Events struct {
	// ShowToast displays a toast with the message.
	ShowToast string `json:"showToast,omitempty"`
	// FormReset resets the form with the given element id.
	FormReset string `json:"formReset,omitempty"`
}

// Trigger fires client-side events via HX-Trigger.
// Values must be ASCII-safe (see headerJSON); the client listens
// for them with document.body.addEventListener.
func Trigger(w http.ResponseWriter, events Events) {
	payload, err := headerJSON(events)
	if err != nil {
		http.Error(w, "Interná chyba servera", http.StatusInternalServerError)

		return
	}

	w.Header().Set("HX-Trigger", payload)
	w.WriteHeader(http.StatusOK)
}

// headerJSON marshals v to JSON with all non-ASCII characters escaped
// as \u sequences, so the result is pure ASCII and safe to send in an
// HTTP header. Browsers decode response headers as Latin-1, so raw
// UTF-8 bytes would arrive in JavaScript as mojibake
// (e.g. "á" becoming "Ã¡").
func headerJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	var out strings.Builder

	for _, r := range string(raw) {
		if r <= unicode.MaxASCII {
			out.WriteRune(r)

			continue
		}

		if r > 0xFFFF {
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&out, "\\u%04x\\u%04x", hi, lo)

			continue
		}

		fmt.Fprintf(&out, "\\u%04x", r)
	}

	return out.String(), nil
}
