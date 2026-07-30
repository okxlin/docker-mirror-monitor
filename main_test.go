package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func writeMonitorConfig(t *testing.T, path, targetURL, token string) {
	t.Helper()

	config := fmt.Sprintf(`server:
  listen: "127.0.0.1:0"
  refresh_interval: 1s
  slow_threshold: 100
  api_token: %q
groups:
  - id: local
    name: Local
    targets:
      - name: Health
        url: %q
        method: GET
        timeout: 100ms
`, token, targetURL)

	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestProbeAllCoalescesConcurrentRuns(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestStarted)
		<-releaseRequest
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeMonitorConfig(t, configPath, target.URL, "test-token")
	config, err := LoadAndValidateConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	monitor := NewMonitor(config, configPath, NewHub())

	firstResult := make(chan bool, 1)
	go func() {
		firstResult <- monitor.ProbeAll()
	}()

	select {
	case <-requestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("first probe did not start")
	}

	if monitor.ProbeAll() {
		t.Fatal("second probe should be coalesced while the first is running")
	}
	close(releaseRequest)
	if !<-firstResult {
		t.Fatal("first probe should report that it ran")
	}
}

func TestReloadConfigAndProbeAllAreRaceFree(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeMonitorConfig(t, configPath, target.URL, "test-token")

	config, err := LoadAndValidateConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	monitor := NewMonitor(config, configPath, NewHub())

	start := make(chan struct{})
	errors := make(chan error, 32)
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			if err := monitor.ReloadConfig(); err != nil {
				errors <- err
			}
		}()
		go func() {
			defer workers.Done()
			<-start
			monitor.ProbeAll()
		}()
	}

	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("reload config: %v", err)
	}
}
