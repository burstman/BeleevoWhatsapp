package queue

import (
	"crypto/tls"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/hibiken/asynq"
)

// Task type names used across the platform. Handlers are registered by phase.
const (
	TaskSendWhatsAppTemplate   = "send:whatsapp_template"
	TaskSyncWhatsAppTemplates  = "sync:whatsapp_templates"
	TaskProcessMetaWebhook     = "process:meta_webhook"
	TaskPurgeMarketingTemplate = "purge:marketing_template"
)

// RedisClientOpt converts a redis:// or rediss:// URL into the options Asynq
// expects. Credentials, db index and TLS are preserved. Use rediss:// for
// services that require encryption (e.g. Upstash), redis:// for plain local
// instances.
func RedisClientOpt(redisURL string) (asynq.RedisClientOpt, error) {
	u, err := url.Parse(redisURL)
	if err != nil {
		return asynq.RedisClientOpt{}, fmt.Errorf("parse redis url: %w", err)
	}

	switch u.Scheme {
	case "redis", "rediss":
	default:
		return asynq.RedisClientOpt{}, fmt.Errorf("unsupported redis url scheme %q", u.Scheme)
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
		opt.TLSConfig = &tls.Config{
			ServerName: u.Hostname(),
			MinVersion: tls.VersionTLS12,
		}
	}

	return opt, nil
}
