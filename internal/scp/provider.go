package scp

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const defaultTTL = 60

type Config struct {
	APIURL       string `env:"SCP_API_URL" default:"https://openapi.samsungsdscloud.com"`
	AccessKey    string `env:"SCP_ACCESS_KEY"`
	SecretKey    string `env:"SCP_SECRET_KEY"`
	ProjectID    string `env:"SCP_PROJECT_ID"`
	ZoneID       string `env:"SCP_ZONE_ID"`
	DomainFilter string `env:"SCP_DOMAIN_FILTER"`
	DryRun       bool   `env:"DRY_RUN" default:"false"`
}

type Provider struct {
	provider.BaseProvider
	client       *Client
	zoneID       string
	domainFilter endpoint.DomainFilter
	dryRun       bool
}

func NewProvider(cfg Config) (*Provider, error) {
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("SCP_ACCESS_KEY and SCP_SECRET_KEY are required")
	}
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("SCP_PROJECT_ID is required")
	}
	if cfg.ZoneID == "" {
		return nil, fmt.Errorf("SCP_ZONE_ID is required")
	}

	p := &Provider{
		client: NewClient(cfg.APIURL, cfg.AccessKey, cfg.SecretKey, cfg.ProjectID),
		zoneID: cfg.ZoneID,
		dryRun: cfg.DryRun,
	}
	if cfg.DomainFilter != "" {
		p.domainFilter = endpoint.NewDomainFilter([]string{cfg.DomainFilter})
	}
	return p, nil
}

func (p *Provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return p.domainFilter
}

func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	records, err := p.client.ListRecords(ctx, p.zoneID)
	if err != nil {
		return nil, fmt.Errorf("list SCP records: %w", err)
	}
	log.Infof("[SCP] Fetched %d records from SCP zone %s", len(records), p.zoneID)

	var endpoints []*endpoint.Endpoint
	for _, r := range records {
		if !provider.SupportedRecordType(r.Type) {
			continue
		}
		ep := endpoint.NewEndpointWithTTL(r.Name, r.Type, endpoint.TTL(r.TTL), r.Records...)
		ep.WithProviderSpecific("scpRecordID", r.ID)
		log.Infof("[SCP] READ: %s \"%s\" → %s (TTL %d)", r.Type, r.Name, strings.Join(r.Records, ", "), r.TTL)
		endpoints = append(endpoints, ep)
	}
	log.Infof("[SCP] Returning %d endpoints to ExternalDNS", len(endpoints))
	return endpoints, nil
}

func (p *Provider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	if changes == nil {
		return nil
	}

	creates := len(changes.Create)
	updates := len(changes.UpdateNew)
	deletes := len(changes.Delete)

	if creates+updates+deletes == 0 {
		log.Infof("[SCP] ApplyChanges: no changes")
		return nil
	}
	log.Infof("[SCP] ApplyChanges: %d create, %d update, %d delete", creates, updates, deletes)

	for _, ep := range changes.Create {
		targets := strings.Join(ep.Targets, ", ")
		if p.dryRun {
			log.Infof("[SCP] [DryRun] CREATE %s \"%s\" → %s (TTL %d)", ep.RecordType, ep.DNSName, targets, ttl(ep))
			continue
		}
		log.Infof("[SCP] CREATE %s \"%s\" → %s (TTL %d)", ep.RecordType, ep.DNSName, targets, ttl(ep))
		if err := p.client.CreateRecord(ctx, p.zoneID, ep.DNSName, ep.RecordType, ep.Targets, ttl(ep)); err != nil {
			return fmt.Errorf("create %s: %w", ep.DNSName, err)
		}
		log.Infof("[SCP] CREATE OK: %s \"%s\"", ep.RecordType, ep.DNSName)
	}

	for i, ep := range changes.UpdateNew {
		old := changes.UpdateOld[i]
		recordID, _ := old.GetProviderSpecificProperty("scpRecordID")
		oldTargets := strings.Join(old.Targets, ", ")
		newTargets := strings.Join(ep.Targets, ", ")
		if p.dryRun {
			log.Infof("[SCP] [DryRun] UPDATE %s \"%s\" [%s] → [%s] (TTL %d)", ep.RecordType, ep.DNSName, oldTargets, newTargets, ttl(ep))
			continue
		}
		log.Infof("[SCP] UPDATE %s \"%s\" [%s] → [%s] (TTL %d)", ep.RecordType, ep.DNSName, oldTargets, newTargets, ttl(ep))
		if recordID == "" {
			if err := p.deleteByName(ctx, old.DNSName, old.RecordType); err != nil {
				return err
			}
			if err := p.client.CreateRecord(ctx, p.zoneID, ep.DNSName, ep.RecordType, ep.Targets, ttl(ep)); err != nil {
				return fmt.Errorf("recreate %s: %w", ep.DNSName, err)
			}
		} else {
			if err := p.client.UpdateRecord(ctx, p.zoneID, recordID, ep.Targets, ttl(ep)); err != nil {
				return fmt.Errorf("update %s: %w", ep.DNSName, err)
			}
		}
		log.Infof("[SCP] UPDATE OK: %s \"%s\" → [%s]", ep.RecordType, ep.DNSName, newTargets)
	}

	for _, ep := range changes.Delete {
		recordID, _ := ep.GetProviderSpecificProperty("scpRecordID")
		targets := strings.Join(ep.Targets, ", ")
		if p.dryRun {
			log.Infof("[SCP] [DryRun] DELETE %s \"%s\" (was: %s)", ep.RecordType, ep.DNSName, targets)
			continue
		}
		log.Infof("[SCP] DELETE %s \"%s\" (was: %s)", ep.RecordType, ep.DNSName, targets)
		if recordID != "" {
			if err := p.client.DeleteRecord(ctx, p.zoneID, recordID); err != nil {
				return fmt.Errorf("delete %s: %w", ep.DNSName, err)
			}
		} else {
			if err := p.deleteByName(ctx, ep.DNSName, ep.RecordType); err != nil {
				return err
			}
		}
		log.Infof("[SCP] DELETE OK: %s \"%s\"", ep.RecordType, ep.DNSName)
	}

	return nil
}

func (p *Provider) deleteByName(ctx context.Context, name, rtype string) error {
	records, err := p.client.ListRecords(ctx, p.zoneID)
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.Name == name && r.Type == rtype {
			if err := p.client.DeleteRecord(ctx, p.zoneID, r.ID); err != nil {
				return fmt.Errorf("delete by name %s: %w", name, err)
			}
		}
	}
	return nil
}

func ttl(ep *endpoint.Endpoint) int32 {
	if ep.RecordTTL.IsConfigured() {
		return int32(ep.RecordTTL)
	}
	return defaultTTL
}
