package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type trackedHTTPFile struct {
	http.File
	closed bool
}

func (f *trackedHTTPFile) Close() error {
	f.closed = true
	return f.File.Close()
}

type trackedFileSystem struct {
	root  string
	files []*trackedHTTPFile
}

func (fs *trackedFileSystem) Open(name string) (http.File, error) {
	file, err := http.Dir(fs.root).Open(name)
	if err != nil {
		return nil, err
	}
	tracked := &trackedHTTPFile{File: file}
	fs.files = append(fs.files, tracked)
	return tracked, nil
}

type failingStatFile struct {
	closed bool
}

func (f *failingStatFile) Close() error                       { f.closed = true; return nil }
func (f *failingStatFile) Read([]byte) (int, error)           { return 0, io.EOF }
func (f *failingStatFile) Seek(int64, int) (int64, error)     { return 0, errors.New("seek") }
func (f *failingStatFile) Readdir(int) ([]os.FileInfo, error) { return nil, errors.New("readdir") }
func (f *failingStatFile) Stat() (os.FileInfo, error)         { return nil, errors.New("stat") }

type failingStatFileSystem struct {
	file *failingStatFile
}

func (fs *failingStatFileSystem) Open(string) (http.File, error) {
	return fs.file, nil
}

func TestNeuteredFileSystemClosesDirectoryAndIndexFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := &trackedFileSystem{root: root}
	wrapped := neuteredFileSystem{fs: fs}

	file, err := wrapped.Open("/")
	if err != nil {
		t.Fatalf("Open directory with index.html: %v", err)
	}
	if file == nil {
		t.Fatal("Open returned nil file")
	}
	_ = file.Close()
	if len(fs.files) != 2 {
		t.Fatalf("tracked opens = %d, want directory and index", len(fs.files))
	}
	for i, file := range fs.files {
		if !file.closed {
			t.Errorf("tracked file %d was not closed", i)
		}
	}

	if err := os.Remove(filepath.Join(root, "index.html")); err != nil {
		t.Fatal(err)
	}
	fs.files = nil
	if file, err := wrapped.Open("/"); err == nil || file != nil {
		t.Fatalf("Open directory without index.html = (%v, %v), want permission error and nil file", file, err)
	}
	if len(fs.files) != 1 || !fs.files[0].closed {
		t.Fatalf("directory file close state = %#v, want one closed file", fs.files)
	}
}

func TestNeuteredFileSystemClosesFileWhenStatFails(t *testing.T) {
	file := &failingStatFile{}
	wrapped := neuteredFileSystem{fs: &failingStatFileSystem{file: file}}
	opened, err := wrapped.Open("/")
	if err == nil || opened != nil {
		t.Fatalf("Open with failing Stat = (%v, %v), want error and nil file", opened, err)
	}
	if !file.closed {
		t.Fatal("file was not closed after Stat failed")
	}
}

func TestSOCKS5DialHonorsRequestCancellation(t *testing.T) {
	for _, withDeadline := range []bool{true, false} {
		name := "explicit cancellation"
		if withDeadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			testSOCKS5DialCancellation(t, withDeadline)
		})
	}
}

func testSOCKS5DialCancellation(t *testing.T, withDeadline bool) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	transport := createProxyTransport("socks5://" + listener.Addr().String())
	ctx, cancel := context.WithCancel(context.Background())
	if withDeadline {
		cancel()
		ctx, cancel = context.WithTimeout(context.Background(), 250*time.Millisecond)
	}
	defer cancel()
	requestDone := make(chan error, 1)
	go func() {
		_, err := transport.DialContext(ctx, "tcp", "registry.example:443")
		requestDone <- err
	}()

	var conn net.Conn
	select {
	case conn = <-accepted:
		defer conn.Close()
	case <-time.After(time.Second):
		t.Fatal("SOCKS proxy did not receive a connection")
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	greeting := make([]byte, 3)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		t.Fatalf("read SOCKS greeting: %v", err)
	}
	if !withDeadline {
		cancel()
	}
	select {
	case err := <-requestDone:
		if err == nil {
			t.Fatal("SOCKS dial unexpectedly succeeded")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SOCKS handshake did not stop after context deadline")
	}

	conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var one [1]byte
	_, err = conn.Read(one[:])
	if err == nil {
		t.Fatal("SOCKS connection stayed readable after request cancellation")
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatal("SOCKS connection remained open after request cancellation")
	}
}

func TestWebSocketOriginThroughTLSTerminatingProxy(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	monitor := NewMonitor(&Config{}, "", hub)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveWebSocket(hub, monitor, w, r)
	}))
	defer server.Close()

	backendAddr := server.Listener.Addr().String()
	dialer := websocket.Dialer{
		NetDialContext: func(context.Context, string, string) (net.Conn, error) {
			return net.Dial("tcp", backendAddr)
		},
		HandshakeTimeout: time.Second,
	}

	connect := func(origin string) (*websocket.Conn, *http.Response, error) {
		dialer := dialer
		return dialer.Dial("ws://public.example/ws", http.Header{
			"Origin":            []string{origin},
			"X-Forwarded-Proto": []string{"https"},
		})
	}

	for _, tt := range []struct {
		name         string
		origin       string
		wantAccepted bool
	}{
		{name: "same host through TLS proxy", origin: "https://public.example", wantAccepted: true},
		{name: "different host", origin: "https://attacker.example"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn, response, err := connect(tt.origin)
			if conn != nil {
				defer conn.Close()
			}
			if response != nil {
				defer response.Body.Close()
			}
			if tt.wantAccepted && err != nil {
				t.Fatalf("same-host HTTPS origin rejected through TLS proxy: %v", err)
			}
			if !tt.wantAccepted && err == nil {
				t.Fatal("cross-origin WebSocket was accepted")
			}
		})
	}
}

func TestPublicFileSystemServesFilesAndRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	public := filepath.Join(root, "public")
	if err := os.Mkdir(public, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(public, "logo.txt"), []byte("public asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("private config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "ssl"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ssl", "key.pem"), []byte("private key"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, link := range []struct{ name, target string }{
		{name: "config-link.yaml", target: "../config.yaml"},
		{name: "ssl-link", target: "../ssl"},
	} {
		if err := os.Symlink(link.target, filepath.Join(public, link.name)); err != nil {
			t.Skipf("symbolic links unavailable: %v", err)
		}
	}

	fs, err := newPublicFileSystem(public)
	if err != nil {
		t.Fatalf("open public filesystem: %v", err)
	}
	defer fs.Close()
	handler := http.StripPrefix("/custom/", http.FileServer(fs))
	for _, tt := range []struct {
		path       string
		wantStatus int
		wantBody   string
		wantDenied bool
	}{
		{path: "/custom/logo.txt", wantStatus: http.StatusOK, wantBody: "public asset"},
		{path: "/custom/", wantStatus: http.StatusForbidden},
		{path: "/custom/config-link.yaml", wantDenied: true},
		{path: "/custom/ssl-link/key.pem", wantDenied: true},
	} {
		t.Run(tt.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if tt.wantDenied {
				if response.Code < 400 {
					t.Fatalf("status = %d, want symlink escape rejected", response.Code)
				}
			} else if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && response.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", response.Body.String(), tt.wantBody)
			}
			if tt.wantBody == "" && (strings.Contains(response.Body.String(), "private config") || strings.Contains(response.Body.String(), "private key")) {
				t.Fatalf("response disclosed private content: %q", response.Body.String())
			}
		})
	}
}

func TestValidateConfigRejectsIncompleteTLS(t *testing.T) {
	for _, tt := range []struct {
		name      string
		cert      string
		key       string
		wantError bool
	}{
		{name: "missing both files", wantError: true},
		{name: "missing private key", cert: "cert.pem", wantError: true},
		{name: "missing certificate", key: "key.pem", wantError: true},
		{name: "both files provided", cert: "cert.pem", key: "key.pem"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := validTestConfig("https://registry.example/v2/")
			config.Server.TLS.Enabled = true
			config.Server.TLS.CertFile = tt.cert
			config.Server.TLS.KeyFile = tt.key
			if err := ValidateConfig(config); (err != nil) != tt.wantError {
				t.Fatalf("ValidateConfig() = %v, want error: %v", err, tt.wantError)
			}
		})
	}
}

func TestProbeSOCKSHandshakeHasBoundedLifetime(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	config := &Config{}
	config.Server.Proxy = "socks5://" + listener.Addr().String()
	monitor := NewMonitor(config, "", NewHub())
	probeDone := make(chan *Result, 1)
	go func() {
		probeDone <- monitor.Probe(Target{
			Name:    "hanging SOCKS proxy",
			URL:     "https://registry.example/v2/",
			Timeout: 30 * time.Millisecond,
		})
	}()

	var conn net.Conn
	select {
	case conn = <-accepted:
		defer conn.Close()
	case <-time.After(time.Second):
		t.Fatal("SOCKS proxy did not receive a connection")
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := io.ReadFull(conn, make([]byte, 3)); err != nil {
		t.Fatalf("read SOCKS greeting: %v", err)
	}
	select {
	case result := <-probeDone:
		if result.Status != "timeout" {
			t.Fatalf("probe status = %q, want timeout", result.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("Probe did not return at its configured timeout")
	}

	// net/http may reuse a dial after the request is cancelled, so the proxy
	// handshake needs its own limit even when the request already timed out.
	conn.SetReadDeadline(time.Now().Add(12 * time.Second))
	var one [1]byte
	if _, err := conn.Read(one[:]); err == nil {
		t.Fatal("unexpected SOCKS proxy data after a timed-out probe")
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatal("SOCKS handshake connection exceeded its 10-second lifetime")
	}
}

func TestPublicFileSystemRejectsDirectoryIndex(t *testing.T) {
	public := t.TempDir()
	if err := os.Mkdir(filepath.Join(public, "index.html"), 0o700); err != nil {
		t.Fatal(err)
	}
	const filename = "visible-by-name.txt"
	if err := os.WriteFile(filepath.Join(public, filename), []byte("public asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs, err := newPublicFileSystem(public)
	if err != nil {
		t.Fatalf("open public filesystem: %v", err)
	}
	defer fs.Close()
	handler := http.StripPrefix("/custom/", http.FileServer(fs))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/custom/", nil))
	if response.Code < 400 {
		t.Errorf("status = %d, want directory index rejected", response.Code)
	}
	if strings.Contains(response.Body.String(), filename) {
		t.Errorf("response listed a directory entry: %q", response.Body.String())
	}
}

func TestNeuteredFileSystemDisallowsReaddirAfterIndexRemoved(t *testing.T) {
	public := t.TempDir()
	index := filepath.Join(public, "index.html")
	if err := os.WriteFile(index, []byte("index page"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrapped := neuteredFileSystem{fs: http.Dir(public)}
	file, err := wrapped.Open("/")
	if err != nil {
		t.Fatalf("Open directory with index.html: %v", err)
	}
	defer file.Close()
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Readdir(-1); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Readdir() = %v, want permission denied after index removal", err)
	}
}
