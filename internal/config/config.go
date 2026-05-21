// Package config loads operator configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// Config holds resolved operator configuration.
type Config struct {
	// GarageS3Endpoint, when set, is written into generated Secrets as
	// AWS_ENDPOINT_URL. Empty means: omit AWS_ENDPOINT_URL.
	GarageS3Endpoint string

	// GarageAdminEndpoint is the base URL for Garage admin API calls.
	GarageAdminEndpoint string

	// GarageAdminToken is the bearer token used by the admin client.
	GarageAdminToken string

	// TraefikEntrypoint is the entrypoint name set on generated IngressRoutes.
	TraefikEntrypoint string

	// TraefikCertResolver, if non-empty, is set on the IngressRoute TLS block.
	TraefikCertResolver string

	// GarageServiceName / Namespace / Port identify the in-cluster Garage
	// Service the IngressRoute should route to.
	GarageServiceName      string
	GarageServiceNamespace string
	GarageServicePort      int
}

// Load reads configuration from the process environment.
func Load() (*Config, error) {
	return loadFrom(os.Getenv)
}

func loadFrom(getenv func(string) string) (*Config, error) {
	host := getenv("GARAGE_HOST")
	s3 := getenv("GARAGE_S3_ENDPOINT")
	admin := getenv("GARAGE_ADMIN_ENDPOINT")

	if host != "" {
		if s3 == "" {
			s3 = fmt.Sprintf("http://%s:3900", host)
		}
		if admin == "" {
			admin = fmt.Sprintf("http://%s:3903", host)
		}
	}

	if admin == "" {
		return nil, errors.New("GARAGE_ADMIN_ENDPOINT (or GARAGE_HOST) is required")
	}
	if _, err := url.Parse(admin); err != nil {
		return nil, fmt.Errorf("GARAGE_ADMIN_ENDPOINT is not a valid URL: %w", err)
	}
	if s3 != "" {
		if _, err := url.Parse(s3); err != nil {
			return nil, fmt.Errorf("GARAGE_S3_ENDPOINT is not a valid URL: %w", err)
		}
	}

	token := getenv("GARAGE_ADMIN_TOKEN")
	if token == "" {
		return nil, errors.New("GARAGE_ADMIN_TOKEN is required")
	}

	entry := getenv("TRAEFIK_ENTRYPOINT")
	if entry == "" {
		entry = "websecure"
	}

	svcName := getenv("GARAGE_SERVICE_NAME")
	if svcName == "" {
		svcName = "garage"
	}
	svcNs := getenv("GARAGE_SERVICE_NAMESPACE")
	if svcNs == "" {
		svcNs = "garage-system"
	}
	svcPort := 3902
	if raw := getenv("GARAGE_WEB_PORT"); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil || p <= 0 || p > 65535 {
			return nil, fmt.Errorf("GARAGE_WEB_PORT must be a TCP port number, got %q", raw)
		}
		svcPort = p
	}

	return &Config{
		GarageS3Endpoint:       s3,
		GarageAdminEndpoint:    admin,
		GarageAdminToken:       token,
		TraefikEntrypoint:      entry,
		TraefikCertResolver:    getenv("TRAEFIK_CERT_RESOLVER"),
		GarageServiceName:      svcName,
		GarageServiceNamespace: svcNs,
		GarageServicePort:      svcPort,
	}, nil
}
