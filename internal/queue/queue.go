package queue

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hibiken/asynq"
)

// Task type names used across the platform. Handlers are registered by phase.
const (
	TaskSendWhatsAppTemplate  = "send:whatsapp_template"
	TaskSyncWhatsAppTemplates = "sync:whatsapp_templates"
	TaskProcessMetaWebhook    = "process:meta_webhook"
)

// RedisClientOpt converts a redis:// or rediss:// URL into the options Asynq
// expects. Credentials and db index are preserved.
func RedisClientOpt(redisURL string) (asynq.RedisClientOpt, error) {
	u, err := url.Parse(redisURL)
	if err != nil {
		return asynq.RedisClientOpt{}, fmt.Errorf("parse redis url: %w", err)
	}

	opt := asynq.RedisClientOpt{
		Addr: u.Host,
	}

	if u.User != nil {
		opt.Username = u.User.Username()
		if pass, ok := u.User.Password(); ok {
			opt.Password = pass
		}
	}

	if db := strings.TrimPrefix(u.Path, "/"); db != "" {
		n, err := strconv.Atoi(db)
		if err != nil {
			return asynq.RedisClientOpt{}, fmt.Errorf("invalid redis db: %q", db)
		}
		opt.DB = n
	}

	if u.Scheme == "rediss" {
		opt.TLSConfig = nil // handled by redis client via tls when set; explicit TLS servers set REDIS_TLS separately
	}

	return opt, nil
}
