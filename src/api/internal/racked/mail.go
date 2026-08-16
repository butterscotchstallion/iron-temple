package racked

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultRelay is the homelab mail relay, the same endpoint and default
// scripts/ci-notify.sh posts CI failures to.
const DefaultRelay = "http://mail-relay.mail.svc.cluster.local/send"

// DefaultRecipient is where recaps go.
const DefaultRecipient = "alerts@homelab.local"

// relayTimeout bounds one POST. The relay owns retry and an on-disk spool, so a
// slow relay is its problem to queue, not ours to wait on — and the reporter
// holds a claimed row while this is in flight.
const relayTimeout = 20 * time.Second

// Mailer posts a rendered recap to the homelab mail relay.
//
// Delivery is an HTTP POST, not SMTP: the relay owns retry and spooling, so a
// mail-server blip queues there instead of being dropped here. That is also why
// this does no retrying of its own — a failed POST is recorded against the
// report_runs row and retried by the next tick, which is a better place for the
// decision than a loop inside one request.
type Mailer struct {
	Endpoint  string
	Recipient string
	Client    *http.Client
}

// NewMailer builds a Mailer, filling in the homelab defaults for empty fields.
func NewMailer(endpoint, recipient string) *Mailer {
	if endpoint == "" {
		endpoint = DefaultRelay
	}
	if recipient == "" {
		recipient = DefaultRecipient
	}
	return &Mailer{
		Endpoint:  endpoint,
		Recipient: recipient,
		Client:    &http.Client{Timeout: relayTimeout},
	}
}

// relayPayload is the relay's request body.
//
// from/name/subject/html are the four fields ci-notify.sh sends and are known
// to work. `to` is sent in addition, so that recaps can be addressed rather
// than landing on whatever the relay defaults to. A relay that ignores unknown
// fields still delivers to its default — the mail arrives either way, which is
// the right way for this to fail while the relay's contract lives in another
// repository.
type relayPayload struct {
	From    string `json:"from"`
	Name    string `json:"name"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

// Send posts one recap. A non-2xx response is an error carrying the status and
// a short prefix of the body, because that is what ends up in report_runs
// .last_error and is all an operator will have to go on.
func (m *Mailer) Send(ctx context.Context, subject, html string) error {
	body, err := json.Marshal(relayPayload{
		From:    "iron-temple",
		Name:    "Iron Temple",
		To:      m.Recipient,
		Subject: subject,
		HTML:    html,
	})
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.Endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.Client.Do(req)
	if err != nil {
		return fmt.Errorf("post to relay: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("relay returned %d: %s", resp.StatusCode, bytes.TrimSpace(snippet))
	}
	return nil
}
