package scp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const defaultTTL = 300

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
	client          *Client
	zoneID          string
	domainFilterStr string
	domainFilter    endpoint.DomainFilter
	dryRun          bool
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
		client:          NewClient(cfg.APIURL, cfg.AccessKey, cfg.SecretKey, cfg.ProjectID),
		zoneID:          cfg.ZoneID,
		domainFilterStr: cfg.DomainFilter,
		dryRun:          cfg.DryRun,
	}
	if cfg.DomainFilter != "" {
		p.domainFilter = endpoint.NewDomainFilter([]string{cfg.DomainFilter})
	}
	return p, nil
}

func (p *Provider) GetDomainFilter() endpoint.DomainFilterInterface {
	return p.domainFilter
}

func (p *Provider) toFQDN(name string) string {
	if p.domainFilterStr == "" || strings.HasSuffix(name, "."+p.domainFilterStr) {
		return name
	}
	return name + "." + p.domainFilterStr
}

func (p *Provider) toSCPName(fqdn string) string {
	if p.domainFilterStr == "" {
		return fqdn
	}
	return strings.TrimSuffix(fqdn, "."+p.domainFilterStr)
}

func (p *Provider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	records, err := p.client.ListRecords(ctx, p.zoneID)
	if err != nil {
		return nil, fmt.Errorf("list SCP records: %w", err)
	}

	var endpoints []*endpoint.Endpoint
	for _, r := range records {
		if !provider.SupportedRecordType(r.Type) {
			continue
		}
		name := p.toFQDN(r.Name)
		var targets []string
		for _, dest := range r.Records {
			targets = append(targets, strings.Split(dest, ";")...)
		}
		sort.Strings(targets)
		ep := endpoint.NewEndpointWithTTL(name, r.Type, endpoint.TTL(r.TTL), targets...)
		ep.WithProviderSpecific("scpRecordID", r.ID)
		log.Debugf("[SCP] read: %s %s → %v", r.Type, name, targets)
		endpoints = append(endpoints, ep)
	}
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
		return nil
	}
	log.Infof("[SCP] apply: %d create, %d update, %d delete", creates, updates, deletes)

	for _, ep := range changes.Create {
		if p.dryRun {
			log.Infof("[SCP] dry-run: create %s %s → %v", ep.RecordType, ep.DNSName, ep.Targets)
			continue
		}
		if err := p.client.CreateRecord(ctx, p.zoneID, p.toSCPName(ep.DNSName), ep.RecordType, ep.Targets, ttl(ep)); err != nil {
			return fmt.Errorf("create %s: %w", ep.DNSName, err)
		}
		log.Infof("[SCP] created %s %s → %v", ep.RecordType, ep.DNSName, ep.Targets)
	}

	for i, ep := range changes.UpdateNew {
		old := changes.UpdateOld[i]
		recordID, _ := old.GetProviderSpecificProperty("scpRecordID")
		if p.dryRun {
			log.Infof("[SCP] dry-run: update %s %s [%v] → [%v]", ep.RecordType, ep.DNSName, old.Targets, ep.Targets)
			continue
		}
		if recordID == "" {
			if err := p.deleteByName(ctx, old.DNSName, old.RecordType); err != nil {
				return err
			}
			if err := p.client.CreateRecord(ctx, p.zoneID, p.toSCPName(ep.DNSName), ep.RecordType, ep.Targets, ttl(ep)); err != nil {
				return fmt.Errorf("recreate %s: %w", ep.DNSName, err)
			}
		} else {
			if err := p.client.UpdateRecord(ctx, p.zoneID, recordID, ep.Targets, ttl(ep)); err != nil {
				return fmt.Errorf("update %s: %w", ep.DNSName, err)
			}
		}
		log.Infof("[SCP] updated %s %s [%v] → [%v]", ep.RecordType, ep.DNSName, old.Targets, ep.Targets)
	}

	for _, ep := range changes.Delete {
		recordID, _ := ep.GetProviderSpecificProperty("scpRecordID")
		if p.dryRun {
			log.Infof("[SCP] dry-run: delete %s %s", ep.RecordType, ep.DNSName)
			continue
		}
		if recordID != "" {
			if err := p.client.DeleteRecord(ctx, p.zoneID, recordID); err != nil {
				return fmt.Errorf("delete %s: %w", ep.DNSName, err)
			}
		} else {
			if err := p.deleteByName(ctx, ep.DNSName, ep.RecordType); err != nil {
				return err
			}
		}
		log.Infof("[SCP] deleted %s %s", ep.RecordType, ep.DNSName)
	}

	return nil
}

func (p *Provider) deleteByName(ctx context.Context, name, rtype string) error {
	scpName := p.toSCPName(name)
	records, err := p.client.ListRecords(ctx, p.zoneID)
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.Name == scpName && r.Type == rtype {
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
