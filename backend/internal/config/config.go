package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr             string
	DataDir          string
	PublicURL        string
	RPID             string
	SessionTTL       time.Duration
	TrustedProxies   []string
	LogLevel         string
	CloudflareAPIURL string
	CookieSecure     bool
}

func Load() (Config, error) {
	cfg := Config{
		Addr:             env("FLAREDNS_ADDR", ":8080"),
		DataDir:          env("FLAREDNS_DATA_DIR", "./data"),
		PublicURL:        strings.TrimRight(env("FLAREDNS_PUBLIC_URL", "http://localhost:8080"), "/"),
		SessionTTL:       12 * time.Hour,
		LogLevel:         env("FLAREDNS_LOG_LEVEL", "info"),
		CloudflareAPIURL: strings.TrimRight(env("FLAREDNS_CLOUDFLARE_API_URL", "https://api.cloudflare.com/client/v4"), "/"),
	}

	if raw := os.Getenv("FLAREDNS_SESSION_TTL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 5*time.Minute {
			return cfg, fmt.Errorf("FLAREDNS_SESSION_TTL must be a duration of at least 5m")
		}
		cfg.SessionTTL = d
	}

	publicURL, err := url.Parse(cfg.PublicURL)
	if err != nil || publicURL.Hostname() == "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") {
		return cfg, fmt.Errorf("FLAREDNS_PUBLIC_URL must be an absolute http or https URL")
	}
	cfg.RPID = env("FLAREDNS_RP_ID", publicURL.Hostname())
	cfg.CookieSecure = publicURL.Scheme == "https"

	if raw := strings.TrimSpace(os.Getenv("FLAREDNS_TRUSTED_PROXIES")); raw != "" {
		for _, proxy := range strings.Split(raw, ",") {
			if proxy = strings.TrimSpace(proxy); proxy != "" {
				cfg.TrustedProxies = append(cfg.TrustedProxies, proxy)
			}
		}
	}
	if raw := os.Getenv("FLAREDNS_COOKIE_SECURE"); raw != "" {
		secure, err := strconv.ParseBool(raw)
		if err != nil {
			return cfg, fmt.Errorf("FLAREDNS_COOKIE_SECURE must be true or false")
		}
		cfg.CookieSecure = secure
	}

	abs, err := filepath.Abs(cfg.DataDir)
	if err != nil {
		return cfg, fmt.Errorf("resolve data directory: %w", err)
	}
	cfg.DataDir = abs
	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
