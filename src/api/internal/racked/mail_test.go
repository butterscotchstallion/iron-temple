package racked

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMailerPostsTheRelayPayload(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("body is not JSON: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewMailer(srv.URL, "alerts@homelab.local")
	if err := m.Send(context.Background(), "Racked: Ada — March 2026", "<p>hi</p>"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// The four fields ci-notify.sh is known to work with, plus the recipient.
	for k, want := range map[string]string{
		"from":    "iron-temple",
		"name":    "Iron Temple",
		"to":      "alerts@homelab.local",
		"subject": "Racked: Ada — March 2026",
		"html":    "<p>hi</p>",
	} {
		if got[k] != want {
			t.Errorf("payload[%q] = %v, want %q", k, got[k], want)
		}
	}
}

func TestMailerDefaults(t *testing.T) {
	m := NewMailer("", "")
	if m.Endpoint != DefaultRelay {
		t.Errorf("endpoint = %q, want the in-cluster relay", m.Endpoint)
	}
	if m.Recipient != DefaultRecipient {
		t.Errorf("recipient = %q, want %q", m.Recipient, DefaultRecipient)
	}
}

// The error is what lands in report_runs.last_error, so it has to say enough to
// diagnose from.
func TestMailerReportsRelayRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream unavailable"))
	}))
	defer srv.Close()

	err := NewMailer(srv.URL, "").Send(context.Background(), "s", "<p>h</p>")
	if err == nil {
		t.Fatal("Send succeeded against a 502")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("error = %v, want the status and the body", err)
	}
}

func TestMailerReportsUnreachableRelay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	if err := NewMailer(url, "").Send(context.Background(), "s", "<p>h</p>"); err == nil {
		t.Fatal("Send succeeded against a closed relay")
	}
}

func TestMailerHonoursContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewMailer(srv.URL, "").Send(ctx, "s", "<p>h</p>"); err == nil {
		t.Fatal("Send succeeded with a cancelled context")
	}
}

// The relay's error body reaches report_runs.last_error, a TEXT column on a UTF8
// server that rejects invalid byte sequences. The bytes are the relay's choice,
// not ours, so they have to be made safe here.
func TestMailerSanitisesTheRelayErrorBody(t *testing.T) {
	// 0xff is never valid UTF-8; the trailing 0xe4 is a 3-byte lead with nothing
	// after it, which is what a byte-limited read of a valid body looks like.
	body := append([]byte("bad "), 0xff, 0xfe, ' ', 0xe4)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	err := NewMailer(srv.URL, "").Send(context.Background(), "s", "<p>h</p>")
	if err == nil {
		t.Fatal("Send succeeded against a 502")
	}
	if !utf8.ValidString(err.Error()) {
		t.Fatalf("error carries invalid UTF-8: %q", err.Error())
	}
	// Still useful: the status and the legible part of the body survive.
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("error lost its diagnostic content: %q", err.Error())
	}
}

func TestErrorSnippet(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"plain", []byte("  upstream unavailable  "), "upstream unavailable"},
		{"invalid bytes dropped", []byte{'o', 'k', 0xff}, "ok"},
		{"truncated rune dropped", []byte{'o', 'k', 0xe4, 0xb8}, "ok"},
		{"empty", []byte{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := errorSnippet(bytes.NewReader(tc.in))
			if got != tc.want {
				t.Fatalf("errorSnippet = %q, want %q", got, tc.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("errorSnippet returned invalid UTF-8: %q", got)
			}
		})
	}
}

// The read limit is a bound on a remote's response, so it has to be in bytes —
// which is exactly how a valid body ends up cut mid-rune.
func TestErrorSnippetCutsALongBodyWithoutBreakingIt(t *testing.T) {
	// Three-byte runes do not divide into 200, so the limit lands mid-character.
	got := errorSnippet(strings.NewReader(strings.Repeat("世", 100)))
	if !utf8.ValidString(got) {
		t.Fatalf("a byte-limited read produced invalid UTF-8: %q", got)
	}
}

// The recap and CI failures go to the same relay, so the two must not drift.
//
// scripts/ci-notify.sh is the older consumer and the one known to work against
// the real service, which makes it the reference: same endpoint, same MAIL_RELAY
// override. Nothing else enforces that, and a relay that moves would otherwise
// be found by one of them silently failing in production.
func TestDefaultRelayMatchesCINotify(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "scripts", "ci-notify.sh"))
	if err != nil {
		t.Skipf("ci-notify.sh not readable from here: %v", err)
	}
	if !strings.Contains(string(script), DefaultRelay) {
		t.Errorf("DefaultRelay %q does not appear in scripts/ci-notify.sh — "+
			"the recap and CI failure mail have drifted apart", DefaultRelay)
	}
	if !strings.Contains(string(script), "MAIL_RELAY") {
		t.Error("ci-notify.sh no longer reads MAIL_RELAY; the override is no longer shared")
	}
}

// The four fields ci-notify.sh sends are known to work against the relay. The
// recap sends those plus `to`, so a field going missing is a regression.
func TestRelayPayloadKeepsTheKnownGoodFields(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := NewMailer(srv.URL, "").Send(context.Background(), "s", "<p>h</p>"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	for _, k := range []string{"from", "name", "subject", "html", "to"} {
		if _, ok := got[k]; !ok {
			t.Errorf("payload is missing %q", k)
		}
	}
}
