package demo_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ujjwal/gopherq/internal/handler/demo"
)

// validateCase is one payload and whether it should pass validation.
type validateCase struct {
	payload string
	wantOK  bool
}

func checkValidate(t *testing.T, validate func(json.RawMessage) error, cases []validateCase) {
	t.Helper()
	for _, tc := range cases {
		err := validate(json.RawMessage(tc.payload))
		if tc.wantOK && err != nil {
			t.Errorf("%s: unexpected error %v", tc.payload, err)
		}
		if !tc.wantOK && err == nil {
			t.Errorf("%s: expected an error, got nil", tc.payload)
		}
	}
}

// ---------- email ----------

func TestValidateEmail(t *testing.T) {
	checkValidate(t, demo.ValidateEmail, []validateCase{
		{`{"to":"a@b.com","subject":"Hi","body":"Hello"}`, true},
		{`{"to":"a@b.com","subject":"Hi"}`, true}, // body is optional
		{`{"subject":"Hi"}`, false},
		{`{"to":"a@b.com"}`, false},
		{`{"to":"  ","subject":"Hi"}`, false},
		{`[1,2,3]`, false},
	})
}

func TestEmailSends(t *testing.T) {
	err := demo.Email(context.Background(), json.RawMessage(`{"to":"a@b.com","subject":"Hi"}`))
	if err != nil {
		t.Fatal(err)
	}
}

func TestEmailStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := demo.Email(ctx, json.RawMessage(`{"to":"a@b.com","subject":"Hi"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// ---------- flaky ----------

func TestValidateFlaky(t *testing.T) {
	checkValidate(t, demo.ValidateFlaky, []validateCase{
		{`{}`, true},
		{`{"fail_rate":0}`, true},
		{`{"fail_rate":1,"duration":"2s"}`, true},
		{`{"fail_rate":1.5}`, false},
		{`{"fail_rate":-0.1}`, false},
		{`{"duration":"abc"}`, false},
		{`{"duration":"-1s"}`, false},
	})
}

func TestFlakyRateZeroNeverFails(t *testing.T) {
	for range 100 {
		if err := demo.Flaky(context.Background(), json.RawMessage(`{"fail_rate":0}`)); err != nil {
			t.Fatalf("fail_rate 0 failed: %v", err)
		}
	}
}

func TestFlakyRateOneAlwaysFails(t *testing.T) {
	for range 100 {
		err := demo.Flaky(context.Background(), json.RawMessage(`{"fail_rate":1}`))
		if !errors.Is(err, demo.ErrSimulatedFailure) {
			t.Fatalf("error = %v, want ErrSimulatedFailure", err)
		}
	}
}

func TestFlakyDurationStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := demo.Flaky(ctx, json.RawMessage(`{"fail_rate":0,"duration":"1h"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

// ---------- webhook ----------

func TestValidateWebhook(t *testing.T) {
	checkValidate(t, demo.ValidateWebhook, []validateCase{
		{`{"url":"https://example.com/hook"}`, true},
		{`{"url":"http://localhost:9000/hook","method":"put","body":{"a":1}}`, true},
		{`{"url":"ftp://example.com/file"}`, false},
		{`{"url":"example.com/hook"}`, false},
		{`{}`, false},
		{`{"url":"https://example.com","method":"TRACE"}`, false},
	})
}

// received records what a test server got from the webhook.
type received struct {
	method      string
	contentType string
	body        string
}

// webhookServer starts a local HTTP server that answers with status and
// records the request it received.
func webhookServer(t *testing.T, status int) (*httptest.Server, *received) {
	t.Helper()
	got := &received{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*got = received{method: r.Method, contentType: r.Header.Get("Content-Type"), body: string(b)}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return srv, got
}

func TestWebhookSendsBody(t *testing.T) {
	srv, got := webhookServer(t, http.StatusOK)
	payload := `{"url":"` + srv.URL + `","method":"put","body":{"event":"signup"}}`

	if err := demo.NewWebhook()(context.Background(), json.RawMessage(payload)); err != nil {
		t.Fatal(err)
	}

	if got.method != http.MethodPut {
		t.Errorf("method = %q, want PUT", got.method)
	}
	if got.contentType != "application/json" {
		t.Errorf("content type = %q, want application/json", got.contentType)
	}
	if got.body != `{"event":"signup"}` {
		t.Errorf("body = %q", got.body)
	}
}

func TestWebhookWithoutBody(t *testing.T) {
	srv, got := webhookServer(t, http.StatusNoContent)
	payload := `{"url":"` + srv.URL + `"}`

	if err := demo.NewWebhook()(context.Background(), json.RawMessage(payload)); err != nil {
		t.Fatal(err)
	}

	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST (the default)", got.method)
	}
	if got.body != "" || got.contentType != "" {
		t.Errorf("expected no body and no content type, got %q / %q", got.body, got.contentType)
	}
}

func TestWebhookNon2xxIsError(t *testing.T) {
	srv, _ := webhookServer(t, http.StatusInternalServerError)
	payload := `{"url":"` + srv.URL + `"}`

	if err := demo.NewWebhook()(context.Background(), json.RawMessage(payload)); err == nil {
		t.Fatal("expected an error for a 500 response, got nil")
	}
}
