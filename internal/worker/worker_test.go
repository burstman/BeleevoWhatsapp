package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"whatsappconverty/internal/whatsapp"
)

type fakeLookup struct {
	active bool
	err    error
	calls  int
	gotID  uuid.UUID
}

func (f *fakeLookup) AutomationEnabled(_ context.Context, id uuid.UUID) (bool, error) {
	f.calls++
	f.gotID = id
	return f.active, f.err
}

// sendPayload is a queued send as the runner hands it to a handler.
func sendPayload(t *testing.T, job whatsapp.SendWhatsAppTemplateJob) []byte {
	t.Helper()
	payload, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return payload
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// A send queued while the automation was on must not go out after a pause, and
// the job itself is what the gate must consult — not any cached trigger state.
func TestGateDropsSendForPausedAutomation(t *testing.T) {
	automationID := uuid.New()
	lookup := &fakeLookup{active: false}
	sent := 0
	handler := gateAutomation(lookup, func(context.Context, []byte) error {
		sent++
		return nil
	}, quietLogger())

	err := handler(context.Background(), sendPayload(t, whatsapp.SendWhatsAppTemplateJob{
		ShopID:       uuid.New(),
		AutomationID: automationID,
		CustomerID:   uuid.New(),
		TemplateID:   uuid.New(),
	}))
	if err != nil {
		t.Fatalf("a dropped send must not fail the task: %v", err)
	}
	if sent != 0 {
		t.Fatalf("sent %d messages for a paused automation, want 0", sent)
	}
	if lookup.calls != 1 || lookup.gotID != automationID {
		t.Fatalf("gate looked up %v (%d calls), want %v once", lookup.gotID, lookup.calls, automationID)
	}
}

func TestGateSendsForActiveAutomation(t *testing.T) {
	lookup := &fakeLookup{active: true}
	sent := 0
	handler := gateAutomation(lookup, func(context.Context, []byte) error {
		sent++
		return nil
	}, quietLogger())

	if err := handler(context.Background(), sendPayload(t, whatsapp.SendWhatsAppTemplateJob{
		AutomationID: uuid.New(),
	})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent %d, want 1", sent)
	}
}

func TestGateIgnoresSendsWithoutAnAutomation(t *testing.T) {
	lookup := &fakeLookup{active: false}
	sent := 0
	handler := gateAutomation(lookup, func(context.Context, []byte) error {
		sent++
		return nil
	}, quietLogger())

	if err := handler(context.Background(), sendPayload(t, whatsapp.SendWhatsAppTemplateJob{
		ShopID: uuid.New(),
	})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent %d, want 1: a send with no automation must pass through", sent)
	}
	if lookup.calls != 0 {
		t.Fatalf("looked an automation up %d times for a non-automation send", lookup.calls)
	}
}

func TestGateSendsWhenTheLookupFails(t *testing.T) {
	lookup := &fakeLookup{err: errors.New("database unavailable")}
	sent := 0
	handler := gateAutomation(lookup, func(context.Context, []byte) error {
		sent++
		return nil
	}, quietLogger())

	if err := handler(context.Background(), sendPayload(t, whatsapp.SendWhatsAppTemplateJob{
		AutomationID: uuid.New(),
	})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if sent != 1 {
		t.Fatal("a failed lookup must not silently swallow a customer message")
	}
}
