package scp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	clientType  = "OpenApi"
	apiLanguage = "en-US"
	basePath    = "/oss2"
)

type Client struct {
	httpClient *http.Client
	apiURL     string
	accessKey  string
	secretKey  string
	projectID  string
}

func NewClient(apiURL, accessKey, secretKey, projectID string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiURL:     strings.TrimRight(apiURL, "/"),
		accessKey:  accessKey,
		secretKey:  secretKey,
		projectID:  projectID,
	}
}

// sign builds the HMAC-SHA256 signature required by SCP OpenAPI.
// StringToSign = method + "\n" + urlPath + "\n" + timestamp + "\n" + accessKey + "\n" + projectId + "\n" + clientType
func (c *Client) sign(method, urlPath, timestamp string) string {
	parts := []string{method, urlPath, timestamp, c.accessKey, c.projectID, clientType}
	mac := hmac.New(sha256.New, []byte(c.secretKey))
	mac.Write([]byte(strings.Join(parts, "\n")))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (c *Client) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	fullPath := basePath + path
	req, err := http.NewRequestWithContext(ctx, method, c.apiURL+fullPath, reqBody)
	if err != nil {
		return err
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Cmp-AccessKey", c.accessKey)
	req.Header.Set("X-Cmp-Signature", c.sign(method, fullPath, timestamp))
	req.Header.Set("X-Cmp-Timestamp", timestamp)
	req.Header.Set("X-Cmp-ClientType", clientType)
	req.Header.Set("X-Cmp-ProjectId", c.projectID)
	req.Header.Set("X-Cmp-Language", apiLanguage)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("SCP API %s %s: status %d: %s", method, path, resp.StatusCode, string(respBytes))
	}
	if out != nil && len(respBytes) > 0 {
		return json.Unmarshal(respBytes, out)
	}
	return nil
}

type DNSRecord struct {
	ID      string   `json:"dnsRecordId"`
	Name    string   `json:"dnsRecordName"`
	Type    string   `json:"dnsRecordType"`
	Records []string `json:"recordDestinations"`
	TTL     int32    `json:"ttl"`
}

type listRecordsResponse struct {
	Contents   []DNSRecord `json:"contents"`
	TotalCount int         `json:"totalCount"`
}

type createRecordRequest struct {
	Name    string   `json:"dnsRecordName"`
	Type    string   `json:"dnsRecordType"`
	Records []string `json:"recordDestinations"`
	TTL     int32    `json:"ttl"`
}

type updateRecordRequest struct {
	Records []string `json:"recordDestinations"`
	TTL     int32    `json:"ttl"`
}

// ListRecords returns all DNS records in the zone.
// GET /oss2/dns/v2/{domainId}/dns-records
func (c *Client) ListRecords(ctx context.Context, zoneID string) ([]DNSRecord, error) {
	path := fmt.Sprintf("/dns/v2/%s/dns-records", zoneID)
	var result listRecordsResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result.Contents, nil
}

// CreateRecord creates a new DNS record in the zone.
// POST /oss2/dns/v2/{domainId}/dns-records
func (c *Client) CreateRecord(ctx context.Context, zoneID, name, recordType string, targets []string, ttl int32) error {
	path := fmt.Sprintf("/dns/v2/%s/dns-records", zoneID)
	return c.do(ctx, http.MethodPost, path, createRecordRequest{
		Name:    name,
		Type:    recordType,
		Records: targets,
		TTL:     ttl,
	}, nil)
}

// UpdateRecord updates targets/TTL of an existing record.
// PUT /oss2/dns/v2/{domainId}/dns-records/{recordId}
func (c *Client) UpdateRecord(ctx context.Context, zoneID, recordID string, targets []string, ttl int32) error {
	path := fmt.Sprintf("/dns/v2/%s/dns-records/%s", zoneID, recordID)
	return c.do(ctx, http.MethodPut, path, updateRecordRequest{
		Records: targets,
		TTL:     ttl,
	}, nil)
}

// DeleteRecord removes a DNS record by ID.
// DELETE /oss2/dns/v2/{domainId}/dns-records/{recordId}
func (c *Client) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	path := fmt.Sprintf("/dns/v2/%s/dns-records/%s", zoneID, recordID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
