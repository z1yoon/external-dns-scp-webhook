package main

import (
	"github.com/codingconcepts/env"
	log "github.com/sirupsen/logrus"
	externaldnsapi "sigs.k8s.io/external-dns/provider/webhook/api"

	"github.com/z1yoon/external-dns-scp-webhook/internal/scp"
	"github.com/z1yoon/external-dns-scp-webhook/internal/server"
)

func main() {
	var serverOpts server.ServerOptions
	if err := env.Set(&serverOpts); err != nil {
		log.Fatalf("failed to load server options: %v", err)
	}

	var cfg scp.Config
	if err := env.Set(&cfg); err != nil {
		log.Fatalf("failed to load SCP config: %v", err)
	}

	p, err := scp.NewProvider(cfg)
	if err != nil {
		log.Fatalf("failed to init SCP provider: %v", err)
	}

	health := &server.HealthServer{}
	health.SetHealthy(true)

	started := make(chan struct{})
	go health.Start(started, serverOpts)
	<-started

	health.SetReady(true)
	log.Infof("starting SCP webhook on %s", serverOpts.GetWebhookAddress())

	externaldnsapi.StartHTTPApi(
		p,
		nil,
		serverOpts.GetReadTimeout(),
		serverOpts.GetWriteTimeout(),
		serverOpts.GetWebhookAddress(),
	)
}
