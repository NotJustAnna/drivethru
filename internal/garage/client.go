// Package garage is a thin HTTP client for the Garage admin API v2.
package garage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is the surface the reconciler depends on. The HTTP-backed
// implementation lives in this package; tests can swap a fake in.
type Client interface {
	EnsureBucket(ctx context.Context, host string) (bucketID string, err error)
	LookupBucket(ctx context.Context, host string) (bucketID string, found bool, err error)
	EnableWebsite(ctx context.Context, bucketID string) error
	EnsureKey(ctx context.Context, name string) (info KeyInfo, created bool, err error)
	LookupKey(ctx context.Context, name string) (accessKeyID string, found bool, err error)
	AllowKeyOnBucket(ctx context.Context, bucketID, accessKeyID string) error
	DenyKeyOnBucket(ctx context.Context, bucketID, accessKeyID string) error
	DeleteBucket(ctx context.Context, bucketID string) error
	DeleteKey(ctx context.Context, accessKeyID string) error
}

// HTTPClient is the live admin-API client.
type HTTPClient struct {
	baseURL string
	token   string
	hc      *http.Client
}

// New constructs an HTTPClient. `endpoint` is the Garage admin base URL.
func New(endpoint, token string) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(endpoint, "/"),
		token:   token,
		hc:      &http.Client{Timeout: 15 * time.Second},
	}
}

// notFoundError is returned by do() on HTTP 404.
type notFoundError struct{ path string }

func (e *notFoundError) Error() string { return "garage: not found at " + e.path }

func isNotFound(err error) bool {
	var nf *notFoundError
	return errors.As(err, &nf)
}

func (c *HTTPClient) do(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("garage: marshal %s: %w", path, err)
		}
		buf = bytes.NewReader(b)
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, buf)
	if err != nil {
		return fmt.Errorf("garage: build %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("garage: call %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &notFoundError{path: path}
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("garage: %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("garage: decode %s: %w", path, err)
	}
	return nil
}

// EnsureBucket looks up a bucket by its globalAlias (the host) and creates it
// if missing. Returns the bucket ID.
func (c *HTTPClient) EnsureBucket(ctx context.Context, host string) (string, error) {
	id, found, err := c.LookupBucket(ctx, host)
	if err != nil {
		return "", err
	}
	if found {
		return id, nil
	}

	var created BucketInfo
	if err := c.do(ctx, http.MethodPost, "/v2/CreateBucket", nil,
		createBucketRequest{GlobalAlias: host}, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", fmt.Errorf("garage: CreateBucket returned empty id")
	}
	return created.ID, nil
}

// LookupBucket returns the bucket ID for the given globalAlias, or found=false
// if no such bucket exists.
func (c *HTTPClient) LookupBucket(ctx context.Context, host string) (string, bool, error) {
	info, err := c.getBucket(ctx, host)
	if err == nil {
		return info.ID, true, nil
	}
	if isNotFound(err) {
		return "", false, nil
	}
	return "", false, err
}

func (c *HTTPClient) getBucket(ctx context.Context, host string) (*BucketInfo, error) {
	q := url.Values{"globalAlias": {host}}
	var info BucketInfo
	if err := c.do(ctx, http.MethodGet, "/v2/GetBucketInfo", q, nil, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// EnableWebsite turns on static-site hosting for the given bucket.
func (c *HTTPClient) EnableWebsite(ctx context.Context, bucketID string) error {
	q := url.Values{"id": {bucketID}}
	body := updateBucketRequest{
		WebsiteAccess: &websiteAccess{
			Enabled:       true,
			IndexDocument: "index.html",
			ErrorDocument: "error.html",
		},
	}
	return c.do(ctx, http.MethodPost, "/v2/UpdateBucket", q, body, nil)
}

// EnsureKey returns existing key info (without secret) if a key with `name`
// exists, otherwise creates a new key and returns its full info (with secret).
// `created` is true only when a fresh key was minted in this call.
func (c *HTTPClient) EnsureKey(ctx context.Context, name string) (KeyInfo, bool, error) {
	existing, err := c.getKey(ctx, name)
	if err == nil {
		return *existing, false, nil
	}
	if !isNotFound(err) {
		return KeyInfo{}, false, err
	}

	var fresh KeyInfo
	if err := c.do(ctx, http.MethodPost, "/v2/CreateKey", nil,
		createKeyRequest{Name: name}, &fresh); err != nil {
		return KeyInfo{}, false, err
	}
	return fresh, true, nil
}

// LookupKey returns the accessKeyID for the key matching `name`, or
// found=false if it doesn't exist.
func (c *HTTPClient) LookupKey(ctx context.Context, name string) (string, bool, error) {
	info, err := c.getKey(ctx, name)
	if err == nil {
		return info.AccessKeyID, true, nil
	}
	if isNotFound(err) {
		return "", false, nil
	}
	return "", false, err
}

func (c *HTTPClient) getKey(ctx context.Context, name string) (*KeyInfo, error) {
	q := url.Values{"search": {name}}
	var info KeyInfo
	if err := c.do(ctx, http.MethodGet, "/v2/GetKeyInfo", q, nil, &info); err != nil {
		return nil, err
	}
	if info.AccessKeyID == "" {
		return nil, &notFoundError{path: "/v2/GetKeyInfo"}
	}
	return &info, nil
}

// AllowKeyOnBucket grants read+write on the bucket to the given access key.
func (c *HTTPClient) AllowKeyOnBucket(ctx context.Context, bucketID, accessKeyID string) error {
	body := allowBucketKeyRequest{
		BucketID:    bucketID,
		AccessKeyID: accessKeyID,
		Permissions: KeyPermissions{Read: true, Write: true, Owner: false},
	}
	return c.do(ctx, http.MethodPost, "/v2/AllowBucketKey", nil, body, nil)
}

// DenyKeyOnBucket revokes read+write on the bucket from the given key.
func (c *HTTPClient) DenyKeyOnBucket(ctx context.Context, bucketID, accessKeyID string) error {
	body := allowBucketKeyRequest{
		BucketID:    bucketID,
		AccessKeyID: accessKeyID,
		Permissions: KeyPermissions{Read: true, Write: true, Owner: true},
	}
	err := c.do(ctx, http.MethodPost, "/v2/DenyBucketKey", nil, body, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// DeleteBucket removes a bucket by ID. Missing buckets are not an error.
func (c *HTTPClient) DeleteBucket(ctx context.Context, bucketID string) error {
	q := url.Values{"id": {bucketID}}
	err := c.do(ctx, http.MethodPost, "/v2/DeleteBucket", q, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// DeleteKey removes a key by access-key ID. Missing keys are not an error.
func (c *HTTPClient) DeleteKey(ctx context.Context, accessKeyID string) error {
	q := url.Values{"id": {accessKeyID}}
	err := c.do(ctx, http.MethodPost, "/v2/DeleteKey", q, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}
