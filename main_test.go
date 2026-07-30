package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReloadRequestAuthorization(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		header     string
		query      string
		want       bool
	}{
		{name: "disabled without configured token"},
		{name: "rejects query token", configured: "secret", query: "secret"},
		{name: "rejects raw authorization token", configured: "secret", header: "secret"},
		{name: "rejects wrong bearer token", configured: "secret", header: "Bearer wrong"},
		{name: "accepts bearer token", configured: "secret", header: "Bearer secret", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/reload?token="+tt.query, nil)
			if tt.header != "" {
				request.Header.Set("Authorization", tt.header)
			}
			if got := reloadRequestAuthorized(request, tt.configured); got != tt.want {
				t.Fatalf("reloadRequestAuthorized() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfigUsesEnvironmentAPIToken(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeMonitorConfig(t, configPath, target.URL, "file-token")
	t.Setenv("DMM_API_TOKEN", "environment-token")

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.Server.APIToken != "environment-token" {
		t.Fatalf("API token = %q, want environment override", config.Server.APIToken)
	}
}

func TestRedactProxyAddress(t *testing.T) {
	tests := map[string]string{
		"":                                      "",
		"https://proxy.example:8443":            "https://proxy.example:8443",
		"https://user:password@proxy.example":   "https://proxy.example",
		"socks5://user:password@127.0.0.1:1080": "socks5://127.0.0.1:1080",
		"://invalid":                            "<invalid>",
	}

	for input, want := range tests {
		if got := redactProxyAddress(input); got != want {
			t.Errorf("redactProxyAddress(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveConfigPath(t *testing.T) {
	tests := []struct {
		name      string
		longPath  string
		shortPath string
		want      string
	}{
		{name: "defaults", longPath: "config.yaml", shortPath: "config.yaml", want: "config.yaml"},
		{name: "long flag", longPath: "long.yaml", shortPath: "config.yaml", want: "long.yaml"},
		{name: "short flag", longPath: "config.yaml", shortPath: "short.yaml", want: "short.yaml"},
		{name: "short flag wins", longPath: "long.yaml", shortPath: "short.yaml", want: "short.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveConfigPath(tt.longPath, tt.shortPath); got != tt.want {
				t.Fatalf("resolveConfigPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

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

func TestProbeClassifiesDeadlineAsTimeout(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer target.Close()

	monitor := NewMonitor(&Config{}, "", NewHub())
	result := monitor.Probe(Target{
		Name:    "slow target",
		URL:     target.URL,
		Method:  http.MethodGet,
		Timeout: 20 * time.Millisecond,
	})

	if result.Status != "timeout" {
		t.Fatalf("probe status = %q, want timeout", result.Status)
	}
	if result.LatencyMs < 0 {
		t.Fatalf("timeout latency = %d, want a measured duration", result.LatencyMs)
	}
}

func TestGroupStatusesIncludePendingTargets(t *testing.T) {
	config := &Config{Groups: []Group{{
		ID:   "registry",
		Name: "Registry",
		Targets: []Target{
			{Name: "first", URL: "https://first.example/v2/", Tags: []string{"one"}},
			{Name: "second", URL: "https://second.example/v2/", Tags: []string{"two"}},
		},
	}}}
	monitor := NewMonitor(config, "", NewHub())

	statuses := monitor.GetGroupStatuses()
	if len(statuses) != 1 {
		t.Fatalf("group count = %d, want 1", len(statuses))
	}
	status := statuses[0]
	if status.Total != 2 || len(status.Results) != 2 {
		t.Fatalf("group total/results = %d/%d, want 2/2", status.Total, len(status.Results))
	}
	for i, result := range status.Results {
		if result.Status != "pending" {
			t.Errorf("result %d status = %q, want pending", i, result.Status)
		}
		if result.LatencyMs != -1 {
			t.Errorf("result %d latency = %d, want -1", i, result.LatencyMs)
		}
		if got := formatTime(result.CheckedAt); got != "--" {
			t.Errorf("result %d formatted time = %q, want --", i, got)
		}
	}
}

func TestValidateConfigRejectsUnsupportedTargetURLs(t *testing.T) {
	for _, targetURL := range []string{
		"registry.example/v2/",
		"ftp://registry.example/v2/",
		"https:///v2/",
	} {
		t.Run(targetURL, func(t *testing.T) {
			config := validTestConfig(targetURL)
			if err := ValidateConfig(config); err == nil {
				t.Fatalf("ValidateConfig() accepted unsupported target URL %q", targetURL)
			}
		})
	}
}

func TestValidateConfigRejectsInvalidProxy(t *testing.T) {
	for _, proxyURL := range []string{
		"proxy.example:8080",
		"ftp://proxy.example:21",
		"http:///missing-host",
	} {
		t.Run(proxyURL, func(t *testing.T) {
			config := validTestConfig("https://registry.example/v2/")
			config.Server.Proxy = proxyURL
			if err := ValidateConfig(config); err == nil {
				t.Fatalf("ValidateConfig() accepted invalid proxy URL %q", proxyURL)
			}
		})
	}
}

func validTestConfig(targetURL string) *Config {
	config := &Config{Groups: []Group{{
		ID:   "registry",
		Name: "Registry",
		Targets: []Target{{
			Name: "target",
			URL:  targetURL,
		}},
	}}}
	config.Server.RefreshInterval = time.Second
	return config
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

func TestQueueProbeRunsAfterActiveProbeForReloadedConfig(t *testing.T) {
	oldRequestStarted := make(chan struct{})
	releaseOldRequest := make(chan struct{})
	oldTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(oldRequestStarted)
		<-releaseOldRequest
		w.WriteHeader(http.StatusOK)
	}))
	defer oldTarget.Close()

	newRequestStarted := make(chan struct{})
	newTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(newRequestStarted)
		w.WriteHeader(http.StatusOK)
	}))
	defer newTarget.Close()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	writeMonitorConfig(t, configPath, oldTarget.URL, "test-token")
	config, err := LoadAndValidateConfig(configPath)
	if err != nil {
		t.Fatalf("load initial config: %v", err)
	}
	monitor := NewMonitor(config, configPath, NewHub())

	activeProbe := make(chan bool, 1)
	go func() {
		activeProbe <- monitor.ProbeAll()
	}()

	select {
	case <-oldRequestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("initial probe did not start")
	}

	writeMonitorConfig(t, configPath, newTarget.URL, "test-token")
	if err := monitor.ReloadConfig(); err != nil {
		t.Fatalf("reload config: %v", err)
	}
	monitor.QueueProbe()
	close(releaseOldRequest)

	if !<-activeProbe {
		t.Fatal("active probe should complete")
	}
	select {
	case <-newRequestStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("queued probe did not use the reloaded config")
	}
}

func TestProbeAllLimitsConcurrentRequests(t *testing.T) {
	const concurrencyLimit = 16

	var current atomic.Int32
	var maximum atomic.Int32
	releaseRequests := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(releaseRequests)
		}
	}()

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		active := current.Add(1)
		defer current.Add(-1)
		for {
			observed := maximum.Load()
			if active <= observed || maximum.CompareAndSwap(observed, active) {
				break
			}
		}
		<-releaseRequests
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	targets := make([]Target, 64)
	for i := range targets {
		targets[i] = Target{
			Name:    fmt.Sprintf("target-%d", i),
			URL:     fmt.Sprintf("%s?id=%d", targetServer.URL, i),
			Method:  http.MethodGet,
			Timeout: 10 * time.Second,
		}
	}
	config := &Config{Groups: []Group{{ID: "test", Name: "Test", Targets: targets}}}
	monitor := NewMonitor(config, "", NewHub())

	probeResult := make(chan bool, 1)
	go func() {
		probeResult <- monitor.ProbeAll()
	}()

	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) && maximum.Load() <= concurrencyLimit {
		time.Sleep(10 * time.Millisecond)
	}

	close(releaseRequests)
	released = true
	if !<-probeResult {
		t.Fatal("probe run should complete")
	}
	if got := maximum.Load(); got > concurrencyLimit {
		t.Fatalf("maximum concurrent probes = %d, want <= %d", got, concurrencyLimit)
	}
}

func TestServeWebSocketSerializesInitialStateAndBroadcasts(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	monitor := NewMonitor(&Config{}, "", hub)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWebSocket(hub, monitor, w, r)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var update StatusUpdate
	if err := conn.ReadJSON(&update); err != nil {
		t.Fatalf("read initial websocket update: %v", err)
	}
	if update.CheckedAt == "" {
		t.Fatal("initial websocket update must include checked_at")
	}

	stopBroadcasts := make(chan struct{})
	defer close(stopBroadcasts)
	go func() {
		for i := 0; ; i++ {
			select {
			case <-stopBroadcasts:
				return
			default:
				hub.Broadcast(&StatusUpdate{CheckedAt: fmt.Sprintf("broadcast-%d", i)})
			}
		}
	}()

	if err := conn.ReadJSON(&update); err != nil {
		t.Fatalf("read broadcast websocket update: %v", err)
	}
	if !strings.HasPrefix(update.CheckedAt, "broadcast-") {
		t.Fatalf("broadcast checked_at = %q", update.CheckedAt)
	}
}
