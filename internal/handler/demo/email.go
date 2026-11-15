package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// emailSendTime is how long the fake email takes to "send".
const emailSendTime = 100 * time.Millisecond

type emailMsg struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func parseEmail(payload json.RawMessage) (emailMsg, error) {
	var m emailMsg
	if err := json.Unmarshal(payload, &m); err != nil {
		return emailMsg{}, fmt.Errorf("email payload: %w", err)
	}
	if strings.TrimSpace(m.To) == "" {
		return emailMsg{}, errors.New(`email needs "to"`)
	}
	if strings.TrimSpace(m.Subject) == "" {
		return emailMsg{}, errors.New(`email needs "subject"`)
	}
	return m, nil
}

// ValidateEmail checks that payload has a "to" and a "subject".
// "body" is optional.
func ValidateEmail(payload json.RawMessage) error {
	_, err := parseEmail(payload)
	return err
}

// Email pretends to send an email: it waits briefly, then logs it.
// No real email is sent.
func Email(ctx context.Context, payload json.RawMessage) error {
	m, err := parseEmail(payload)
	if err != nil {
		return err
	}

	if err := sleep(ctx, emailSendTime); err != nil {
		return err
	}

	log.Printf("demo email: %q sent to %s", m.Subject, m.To)
	return nil
}
