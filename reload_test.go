package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestReloadDiscardsResultsForChangedTarget(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeMonitorConfig(t, configPath, "https://old.example/v2/", "test-token")
	config, err := LoadAndValidateConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	monitor := NewMonitor(config, configPath, NewHub())
	monitor.results["local"]["Health"] = &Result{
		Name: "Health", URL: "https://old.example", Status: "healthy",
	}
	writeMonitorConfig(t, configPath, "https://new.example/v2/", "test-token")
	if err := monitor.ReloadConfig(); err != nil {
		t.Fatal(err)
	}
	result := monitor.GetGroupStatuses()[0].Results[0]
	if result.URL != "https://new.example" || result.Status != "pending" {
		t.Fatalf("changed target retained an old probe: %+v", result)
	}
}

func TestReloadIgnoresProbeFromPreviousConfig(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	defer close(release)

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeMonitorConfig(t, configPath, server.URL, "test-token")
	config, err := LoadAndValidateConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config.Groups[0].Targets[0].Timeout = 5 * time.Second
	monitor := NewMonitor(config, configPath, NewHub())
	finished := make(chan bool, 1)
	go func() { finished <- monitor.ProbeAll() }()
	select {
	case <-started:
	case <-time.After(4 * time.Second):
		t.Fatal("probe did not start")
	}
	writeMonitorConfig(t, configPath, "https://new.example/v2/", "test-token")
	if err := monitor.ReloadConfig(); err != nil {
		t.Fatal(err)
	}
	// Let the request complete before inspecting the result store.
	release <- struct{}{}
	if !<-finished {
		t.Fatal("probe did not run")
	}
	result := monitor.GetGroupStatuses()[0].Results[0]
	if result.URL != "https://new.example" || result.Status != "pending" {
		t.Fatalf("obsolete in-flight probe overwrote reloaded target: %+v", result)
	}
}

func TestReloadPreservesUnchangedTarget(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeMonitorConfig(t, configPath, "https://registry.example/v2/", "test-token")
	config, err := LoadAndValidateConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	monitor := NewMonitor(config, configPath, NewHub())
	want := &Result{Name: "Health", URL: "https://registry.example", Status: "healthy"}
	monitor.results["local"]["Health"] = want
	if err := monitor.ReloadConfig(); err != nil {
		t.Fatal(err)
	}
	if got := monitor.GetGroupStatuses()[0].Results[0]; got != want {
		t.Fatalf("unchanged target lost its probe: %+v", got)
	}
}

func TestReloadDiscardsResultsForChangedProbeSettings(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"method":     func(c *Config) { c.Groups[0].Targets[0].Method = "HEAD" },
		"timeout":    func(c *Config) { c.Groups[0].Targets[0].Timeout = time.Second },
		"tags":       func(c *Config) { c.Groups[0].Targets[0].Tags = []string{"new"} },
		"proxy":      func(c *Config) { c.Server.Proxy = "http://127.0.0.1:1080" },
		"user agent": func(c *Config) { c.Server.UserAgent = "new-agent" },
		"threshold":  func(c *Config) { c.Server.SlowThreshold++ },
	} {
		t.Run(name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			writeMonitorConfig(t, configPath, "https://registry.example/v2/", "test-token")
			config, err := LoadAndValidateConfig(configPath)
			if err != nil {
				t.Fatal(err)
			}
			monitor := NewMonitor(config, configPath, NewHub())
			monitor.results["local"]["Health"] = &Result{Name: "Health", Status: "healthy"}
			updated, err := LoadAndValidateConfig(configPath)
			if err != nil {
				t.Fatal(err)
			}
			change(updated)
			writeReloadConfig(t, configPath, updated)
			if err := monitor.ReloadConfig(); err != nil {
				t.Fatal(err)
			}
			if got := monitor.GetGroupStatuses()[0].Results[0]; got.Status != "pending" {
				t.Fatalf("changed probe settings retained old result: %+v", got)
			}
		})
	}
}

func TestReloadRejectsListenerChanges(t *testing.T) {
	for name, change := range map[string]func(*Config){
		"address": func(c *Config) { c.Server.Listen = "127.0.0.1:9443" },
		"TLS": func(c *Config) {
			c.Server.TLS.Enabled = true
			c.Server.TLS.CertFile = "cert.pem"
			c.Server.TLS.KeyFile = "key.pem"
		},
	} {
		t.Run(name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			writeMonitorConfig(t, configPath, "https://registry.example/v2/", "test-token")
			config, err := LoadAndValidateConfig(configPath)
			if err != nil {
				t.Fatal(err)
			}
			monitor := NewMonitor(config, configPath, NewHub())
			result := &Result{Name: "Health", Status: "healthy"}
			monitor.results["local"]["Health"] = result
			transport := monitor.httpClient.Transport
			updated, err := LoadAndValidateConfig(configPath)
			if err != nil {
				t.Fatal(err)
			}
			change(updated)
			updated.Server.Proxy = "http://127.0.0.1:1080"
			writeReloadConfig(t, configPath, updated)
			if err := monitor.ReloadConfig(); err == nil {
				t.Fatal("reload accepted a listener change without restarting the server")
			}
			if monitor.GetConfig() != config || monitor.httpClient.Transport != transport || monitor.results["local"]["Health"] != result {
				t.Fatal("rejected reload changed active configuration, transport or results")
			}
		})
	}
}

func writeReloadConfig(t *testing.T, path string, config *Config) {
	t.Helper()
	data, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
