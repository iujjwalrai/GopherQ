package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/ujjwal/gopherq/internal/handler"
)

// webhookTimeout caps one webhook call, even if ctx has no deadline.
const webhookTimeout = 10 * time.Second

var webhookMethods = []string{
	http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
}

type webhookParams struct {
	URL    string          `json:"url"`
	Method string          `json:"method"`
	Body   json.RawMessage `json:"body"`
}

// parseWebhook reads and checks the payload. The returned Method is
// always uppercase, and POST when none was given.
//
// Security note: any http(s) URL is allowed, including localhost and
// private addresses. A public deployment must block those (SSRF).
func parseWebhook(payload json.RawMessage) (webhookParams, error) {
	var p webhookParams
	if err := json.Unmarshal(payload, &p); err != nil {
		return webhookParams{}, fmt.Errorf("webhook payload: %w", err)
	}

	u, err := url.Parse(p.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return webhookParams{}, fmt.Errorf("webhook url must be http(s), got %q", p.URL)
	}

	p.Method = strings.ToUpper(p.Method)
	if p.Method == "" {
		p.Method = http.MethodPost
	}
	if !slices.Contains(webhookMethods, p.Method) {
		return webhookParams{}, fmt.Errorf("webhook method must be one of %s, got %q",
			strings.Join(webhookMethods, " "), p.Method)
	}

	return p, nil
}

// ValidateWebhook checks "url" (http or https), "method" (GET, POST, PUT,
// PATCH or DELETE in any case, default POST) and the optional "body".
func ValidateWebhook(payload json.RawMessage) error {
	_, err := parseWebhook(payload)
	return err
}

// NewWebhook returns a handler that sends "body" as JSON to "url" using
// "method". Any 2xx response is success; anything else is an error.
func NewWebhook() handler.HandlerFunc {
	client := &http.Client{Timeout: webhookTimeout}

	return func(ctx context.Context, payload json.RawMessage) error {
		p, err := parseWebhook(payload)
		if err != nil {
			return err
		}

		// Leave body as a nil io.Reader when there is nothing to send.
		var body io.Reader
		if len(p.Body) > 0 {
			body = bytes.NewReader(p.Body)
		}

		req, err := http.NewRequestWithContext(ctx, p.Method, p.URL, body)
		if err != nil {
			return fmt.Errorf("webhook: %w", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("webhook: %w", err)
		}
		defer resp.Body.Close()

		// Read (and throw away) the response so the connection can be reused.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return errors.New("webhook: " + p.Method + " " + p.URL + " returned " + resp.Status)
		}
		return nil
	}
}
