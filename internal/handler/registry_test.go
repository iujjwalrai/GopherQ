package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ujjwal/gopherq/internal/handler"
)

func noop(context.Context, json.RawMessage) error { return nil }

func TestRegisterAndHas(t *testing.T) {
	reg := handler.NewRegistry()
	reg.Register("email", noop, nil)

	if !reg.Has("email") {
		t.Fatal(`Has("email") = false, want true`)
	}
	if reg.Has("emial") {
		t.Fatal(`Has("emial") = true, want false`)
	}
}

func TestValidate(t *testing.T) {
	errBad := errors.New("needs \"to\"")

	reg := handler.NewRegistry()
	reg.Register("anything", noop, nil)
	reg.Register("strict", noop, func(json.RawMessage) error { return errBad })

	payload := json.RawMessage(`{}`)

	if err := reg.Validate("anything", payload); err != nil {
		t.Errorf("nil validator: error = %v, want nil", err)
	}
	if err := reg.Validate("strict", payload); !errors.Is(err, errBad) {
		t.Errorf("strict: error = %v, want %v", err, errBad)
	}
	if err := reg.Validate("missing", payload); !errors.Is(err, handler.ErrUnknownType) {
		t.Errorf("missing: error = %v, want ErrUnknownType", err)
	}
}

func TestRegisterPanicsOnMistakes(t *testing.T) {
	tests := []struct {
		name string
		do   func(reg *handler.Registry)
	}{
		{name: "empty type", do: func(reg *handler.Registry) { reg.Register("", noop, nil) }},
		{name: "nil handler", do: func(reg *handler.Registry) { reg.Register("email", nil, nil) }},
		{name: "duplicate", do: func(reg *handler.Registry) {
			reg.Register("email", noop, nil)
			reg.Register("email", noop, nil)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected a panic, got none")
				}
			}()

			tc.do(handler.NewRegistry())
		})
	}
}
