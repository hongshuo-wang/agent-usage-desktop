package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/config"
)

const minimumSettingsInterval = 10 * time.Second

type collectorSetting struct {
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	Paths        []string `json:"paths"`
	ScanInterval string   `json:"scan_interval"`
}

type collectorSettingsResponse struct {
	Collectors          []collectorSetting `json:"collectors"`
	PricingSyncInterval string             `json:"pricing_sync_interval"`
}

func collectorSettingsFromConfig(cfg *config.Config) collectorSettingsResponse {
	collectors := []struct {
		name   string
		config config.CollectorConfig
	}{
		{"claude", cfg.Collectors.Claude},
		{"codex", cfg.Collectors.Codex},
		{"openclaw", cfg.Collectors.OpenClaw},
		{"opencode", cfg.Collectors.OpenCode},
	}
	response := collectorSettingsResponse{
		Collectors:          make([]collectorSetting, 0, len(collectors)),
		PricingSyncInterval: cfg.Pricing.SyncInterval.String(),
	}
	for _, collector := range collectors {
		response.Collectors = append(response.Collectors, collectorSetting{
			Name: collector.name, Enabled: collector.config.Enabled,
			Paths: collector.config.Paths, ScanInterval: collector.config.ScanInterval.String(),
		})
	}
	return response
}

func (s *Server) handleCollectorSettingsGet(w http.ResponseWriter, _ *http.Request) {
	if s.configPath == "" {
		http.Error(w, "config path is not configured", http.StatusServiceUnavailable)
		return
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	cfg, err := config.Load(s.configPath)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, collectorSettingsFromConfig(cfg))
}

func (s *Server) handleCollectorSettingsPut(w http.ResponseWriter, r *http.Request) {
	if s.configPath == "" {
		http.Error(w, "config path is not configured", http.StatusServiceUnavailable)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	var request collectorSettingsResponse
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "settings body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		badRequest(w, fmt.Errorf("invalid settings: %w", err))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "settings body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		badRequest(w, fmt.Errorf("invalid settings: multiple JSON values"))
		return
	}
	updates, pricingInterval, err := validateCollectorSettings(request)
	if err != nil {
		badRequest(w, err)
		return
	}

	s.configMu.Lock()
	defer s.configMu.Unlock()
	cfg, err := config.Load(s.configPath)
	if err != nil {
		serverError(w, err)
		return
	}
	cfg.Collectors.Claude = updates["claude"]
	cfg.Collectors.Codex = updates["codex"]
	cfg.Collectors.OpenClaw = updates["openclaw"]
	cfg.Collectors.OpenCode = updates["opencode"]
	cfg.Pricing.SyncInterval = pricingInterval
	if err := config.Save(s.configPath, cfg); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, map[string]bool{"restart_required": true})
}

func validateCollectorSettings(request collectorSettingsResponse) (map[string]config.CollectorConfig, time.Duration, error) {
	pricingInterval, err := parseSettingsInterval("pricing_sync_interval", request.PricingSyncInterval)
	if err != nil {
		return nil, 0, err
	}
	if len(request.Collectors) != 4 {
		return nil, 0, fmt.Errorf("collectors must contain claude, codex, openclaw, and opencode exactly once")
	}
	updates := make(map[string]config.CollectorConfig, 4)
	for _, collector := range request.Collectors {
		switch collector.Name {
		case "claude", "codex", "openclaw", "opencode":
		default:
			return nil, 0, fmt.Errorf("unknown collector %q", collector.Name)
		}
		if _, exists := updates[collector.Name]; exists {
			return nil, 0, fmt.Errorf("duplicate collector %q", collector.Name)
		}
		if len(collector.Paths) == 0 {
			return nil, 0, fmt.Errorf("collector %q paths must not be empty", collector.Name)
		}
		paths := make([]string, len(collector.Paths))
		for index, path := range collector.Paths {
			path = strings.TrimSpace(path)
			if path == "" {
				return nil, 0, fmt.Errorf("collector %q paths must not contain empty values", collector.Name)
			}
			paths[index] = path
		}
		interval, err := parseSettingsInterval("collector "+collector.Name+" scan_interval", collector.ScanInterval)
		if err != nil {
			return nil, 0, err
		}
		updates[collector.Name] = config.CollectorConfig{
			Enabled: collector.Enabled, Paths: paths, ScanInterval: interval,
		}
	}
	return updates, pricingInterval, nil
}

func parseSettingsInterval(field, value string) (time.Duration, error) {
	interval, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration", field)
	}
	if interval < minimumSettingsInterval {
		return 0, fmt.Errorf("%s must be at least %s", field, minimumSettingsInterval)
	}
	return interval, nil
}
