package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"whatsappconverty/internal/queue"
)

// SendWhatsAppTemplateJob is the queue payload for an automated send: orders
// events and delivery events materialize into this job and the worker runs the
// full tenant-scoped send gate. It carries the same fields as SendRequest so a
// worker has everything needed without re-deriving context.
type SendWhatsAppTemplateJob struct {
	ShopID          uuid.UUID         `json:"shop_id"`
	CustomerID      uuid.UUID         `json:"customer_id"`
	TemplateID      uuid.UUID         `json:"template_id"`
	ConvertyOrderID string            `json:"converty_order_id"`
	Purpose         string            `json:"purpose"`
	Variables       map[string]string `json:"variables"`
	IdempotencyKey  string            `json:"idempotency_key"`
}

// EnqueueSend places a send on the queue and reports whether it was accepted.
// When no asynq client exists (no Redis), it returns queued=false and the
// caller falls back to an inline SendTemplateMessage so automation keeps
// working in degraded mode.
func (s *Service) EnqueueSend(ctx context.Context, job SendWhatsAppTemplateJob) (queued bool, err error) {
	return s.EnqueueSendAt(ctx, job, time.Time{})
}

// EnqueueSendAt is EnqueueSend with an optional absolute delivery moment. A
// zero time sends immediately; otherwise the task is parked in Redis until the
// worker clock reaches it (survives restarts). Times already in the past are
// sent immediately.
func (s *Service) EnqueueSendAt(ctx context.Context, job SendWhatsAppTemplateJob, at time.Time) (queued bool, err error) {
	if s.queue == nil {
		return false, nil
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return false, err
	}
	task := asynq.NewTask(queue.TaskSendWhatsAppTemplate, payload)
	opts := []asynq.Option{
		asynq.Queue("critical"),
		asynq.MaxRetry(5),
		asynq.Timeout(60 * time.Second),
		asynq.Retention(24 * time.Hour),
	}
	if !at.IsZero() && at.After(time.Now()) {
		opts = append(opts, asynq.ProcessAt(at))
	}
	_, err = s.queue.Enqueue(task, opts...)
	if err != nil {
		return false, err
	}
	return true, nil
}

// HandleSendWhatsAppTemplate is the worker handler for automated sends. Sends
// refused by the send gate (*SendRejection) are logged and not retried — the
// message row already records the failure — while transient infrastructure
// errors bubble up so asynq retries them.
func (s *Service) HandleSendWhatsAppTemplate(ctx context.Context, task *asynq.Task) error {
	var job SendWhatsAppTemplateJob
	if err := json.Unmarshal(task.Payload(), &job); err != nil {
		return err
	}

	_, err := s.SendTemplateMessage(ctx, SendRequest{
		ShopID:          job.ShopID,
		CustomerID:      job.CustomerID,
		TemplateID:      job.TemplateID,
		ConvertyOrderID: job.ConvertyOrderID,
		Purpose:         job.Purpose,
		Variables:       job.Variables,
		IdempotencyKey:  job.IdempotencyKey,
	})
	var rej *SendRejection
	if errors.As(err, &rej) {
		s.log.Warn("automation send rejected, no retry",
			"shop_id", job.ShopID, "customer_id", job.CustomerID,
			"template_id", job.TemplateID, "code", rej.Code, "reason", rej.Reason)
		return nil
	}
	return err
}
