package racked

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
