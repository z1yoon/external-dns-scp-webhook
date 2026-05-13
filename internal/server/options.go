package server

import (
	"fmt"
	"time"
)

type ServerOptions struct {
	WebhookHost  string `env:"WEBHOOK_HOST" default:"localhost"`
	WebhookPort  uint16 `env:"WEBHOOK_PORT" default:"8888"`
	HealthHost   string `env:"HEALTH_HOST" default:"0.0.0.0"`
	HealthPort   uint16 `env:"HEALTH_PORT" default:"8080"`
	ReadTimeout  int    `env:"READ_TIMEOUT" default:"60000"`
	WriteTimeout int    `env:"WRITE_TIMEOUT" default:"60000"`
}

func (o ServerOptions) GetWebhookAddress() string {
	return fmt.Sprintf("%s:%d", o.WebhookHost, o.WebhookPort)
}

func (o ServerOptions) GetHealthAddress() string {
	return fmt.Sprintf("%s:%d", o.HealthHost, o.HealthPort)
}

func (o ServerOptions) GetReadTimeout() time.Duration {
	return time.Duration(o.ReadTimeout) * time.Millisecond
}

func (o ServerOptions) GetWriteTimeout() time.Duration {
	return time.Duration(o.WriteTimeout) * time.Millisecond
}
