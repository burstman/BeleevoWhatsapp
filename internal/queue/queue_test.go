package queue

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"whatsappconverty/internal/database"
)

func TestEnqueueRejectsIncompleteJobs(t *testing.T) {
	var db database.Querier // a nil handle is what the validation below must refuse
	payload := []byte(`{"shop_id":"x"}`)

	cases := map[string]Params{
		"no kind":     {ShopID: uuid.New(), Payload: payload},
		"no shop":     {Kind: TaskSendWhatsAppTemplate, Payload: payload},
		"no payload":  {Kind: TaskSendWhatsAppTemplate, ShopID: uuid.New()},
		"no database": {Kind: TaskSendWhatsAppTemplate, ShopID: uuid.New(), Payload: payload},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Enqueue(context.Background(), db, p); err == nil {
				t.Fatal("a job that cannot be stored must be refused")
			}
		})
	}
}

func TestPermanentWrapsAndIsDetected(t *testing.T) {
	if Permanent(nil) != nil {
		t.Fatal("Permanent(nil) must stay nil, not turn a success into a failure")
	}

	sentinel := errors.New("gate refused the message")
	wrapped := Permanent(sentinel)
	if !IsPermanent(wrapped) {
		t.Fatal("a permanent error must be detectable")
	}
	if !errors.Is(wrapped, sentinel) {
		t.Fatal("Permanent must keep the original error readable")
	}
	if IsPermanent(sentinel) {
		t.Fatal("an unwrapped error must not look permanent")
	}
	if !IsPermanent(errors.Join(errors.New("later"), wrapped)) {
		t.Fatal("permanence must survive being wrapped again on the way up")
	}
}

func TestDecideCoversTheRetryPolicy(t *testing.T) {
	job := Job{Attempts: 1, MaxAttempts: 5}

	if got := decide(nil, job); got.kind != actionComplete {
		t.Fatalf("a success must finish the job, got %v", got.kind)
	}
	if got := decide(Permanent(errors.New("gate refused")), job); got.kind != actionDrop {
		t.Fatal("a refusal must be dropped, not retried into the same refusal")
	}

	transient := errors.New("meta timeout")
	got := decide(transient, job)
	if got.kind != actionRetry {
		t.Fatal("a transient failure must be retried")
	}
	if got.retryIn != retryBase {
		t.Fatalf("first retry waits %s, want %s", got.retryIn, retryBase)
	}
	if !errors.Is(got.cause, transient) {
		t.Fatal("the retry must remember why it failed")
	}

	spent := decide(transient, Job{Attempts: 5, MaxAttempts: 5})
	if spent.kind != actionDrop {
		t.Fatal("a job that used every attempt must be dropped")
	}
}

func TestBackoffDoublesAndCaps(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{attempts: 0, want: retryBase},
		{attempts: 1, want: retryBase},
		{attempts: 2, want: 2 * retryBase},
		{attempts: 3, want: 4 * retryBase},
		{attempts: 4, want: 8 * retryBase},
		{attempts: 99, want: retryCap},
	}
	for _, tc := range cases {
		if got := backoff(tc.attempts); got != tc.want {
			t.Fatalf("backoff(%d) = %s, want %s", tc.attempts, got, tc.want)
		}
	}
}

func TestRunnerResolvesHandlersByKind(t *testing.T) {
	runner := NewRunner(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if runner.handler("nope") != nil {
		t.Fatal("an unregistered kind must have no handler")
	}

	called := false
	runner.Handle(TaskPurgeMarketingTemplate, func(context.Context, []byte) error {
		called = true
		return nil
	})
	h := runner.handler(TaskPurgeMarketingTemplate)
	if h == nil {
		t.Fatal("a registered kind must resolve to its handler")
	}
	if err := h(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !called {
		t.Fatal("the registered handler must be the one that runs")
	}
}
