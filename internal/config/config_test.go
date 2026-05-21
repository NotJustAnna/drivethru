package config

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad_GarageHostDerivesEndpoints(t *testing.T) {
	cfg, err := loadFrom(env(map[string]string{
		"GARAGE_HOST":        "garage.svc",
		"GARAGE_ADMIN_TOKEN": "tok",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GarageS3Endpoint != "http://garage.svc:3900" {
		t.Errorf("s3 endpoint: got %q", cfg.GarageS3Endpoint)
	}
	if cfg.GarageAdminEndpoint != "http://garage.svc:3903" {
		t.Errorf("admin endpoint: got %q", cfg.GarageAdminEndpoint)
	}
}

func TestLoad_OverrideEndpointsIndividually(t *testing.T) {
	cfg, err := loadFrom(env(map[string]string{
		"GARAGE_HOST":           "garage.svc",
		"GARAGE_S3_ENDPOINT":    "http://s3.example:3900",
		"GARAGE_ADMIN_ENDPOINT": "http://admin.example:3903",
		"GARAGE_ADMIN_TOKEN":    "tok",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GarageS3Endpoint != "http://s3.example:3900" {
		t.Errorf("s3 endpoint not overridden: %q", cfg.GarageS3Endpoint)
	}
	if cfg.GarageAdminEndpoint != "http://admin.example:3903" {
		t.Errorf("admin endpoint not overridden: %q", cfg.GarageAdminEndpoint)
	}
}

func TestLoad_AdminEndpointRequired(t *testing.T) {
	_, err := loadFrom(env(map[string]string{"GARAGE_ADMIN_TOKEN": "tok"}))
	if err == nil || !strings.Contains(err.Error(), "GARAGE_ADMIN_ENDPOINT") {
		t.Fatalf("expected missing admin endpoint error, got %v", err)
	}
}

func TestLoad_TokenRequired(t *testing.T) {
	_, err := loadFrom(env(map[string]string{"GARAGE_HOST": "garage.svc"}))
	if err == nil || !strings.Contains(err.Error(), "GARAGE_ADMIN_TOKEN") {
		t.Fatalf("expected missing token error, got %v", err)
	}
}

func TestLoad_S3EndpointOmittedWhenOnlyAdminSet(t *testing.T) {
	cfg, err := loadFrom(env(map[string]string{
		"GARAGE_ADMIN_ENDPOINT": "http://admin.example:3903",
		"GARAGE_ADMIN_TOKEN":    "tok",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GarageS3Endpoint != "" {
		t.Errorf("expected empty S3 endpoint, got %q", cfg.GarageS3Endpoint)
	}
}

func TestLoad_TraefikDefaults(t *testing.T) {
	cfg, err := loadFrom(env(map[string]string{
		"GARAGE_HOST":        "garage.svc",
		"GARAGE_ADMIN_TOKEN": "tok",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TraefikEntrypoint != "websecure" {
		t.Errorf("entrypoint default: got %q", cfg.TraefikEntrypoint)
	}
	if cfg.TraefikCertResolver != "" {
		t.Errorf("certResolver should be empty by default: %q", cfg.TraefikCertResolver)
	}
}

func TestLoad_GarageWebPortInvalid(t *testing.T) {
	cases := []string{"abc", "0", "70000", "-1"}
	for _, p := range cases {
		_, err := loadFrom(env(map[string]string{
			"GARAGE_HOST":        "g",
			"GARAGE_ADMIN_TOKEN": "tok",
			"GARAGE_WEB_PORT":    p,
		}))
		if err == nil {
			t.Errorf("GARAGE_WEB_PORT=%q should error", p)
		}
	}
}

func TestLoad_ServiceDefaults(t *testing.T) {
	cfg, err := loadFrom(env(map[string]string{
		"GARAGE_HOST":        "g",
		"GARAGE_ADMIN_TOKEN": "tok",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GarageServiceName != "garage" || cfg.GarageServiceNamespace != "garage-system" || cfg.GarageServicePort != 3902 {
		t.Errorf("service defaults wrong: %+v", cfg)
	}
}
