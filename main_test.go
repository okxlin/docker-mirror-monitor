package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
