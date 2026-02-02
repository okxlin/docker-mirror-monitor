package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/net/proxy"
	"gopkg.in/yaml.v3"
)

// TagColor 标签颜色配置
type TagColor struct {
	Keyword string `yaml:"keyword" json:"keyword"`
	Color   string `yaml:"color" json:"color"`
	BgColor string `yaml:"bg_color" json:"bg_color"`
}

// Site 站点配置
type Site struct {
	Logo         string `yaml:"logo" json:"logo"`
	LogoIcon     string `yaml:"logo_icon" json:"logo_icon"`
	Title        string `yaml:"title" json:"title"`
	Description  string `yaml:"description" json:"description"`
	BrowserTitle string `yaml:"browser_title" json:"browser_title"`
	Favicon      string `yaml:"favicon" json:"favicon"`
}

// FooterLink 页脚链接
type FooterLink struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}

// Footer 页脚配置
type Footer struct {
	Text       string       `yaml:"text" json:"text"`
	ICP        string       `yaml:"icp" json:"icp"`
	ICPURL     string       `yaml:"icp_url" json:"icp_url"`
	Copyright  string       `yaml:"copyright" json:"copyright"`
	EnableHTML bool         `yaml:"enable_html" json:"enable_html"`
	Links      []FooterLink `yaml:"links" json:"links"`
}

// SiteNotice 顶部公告/通知配置
type SiteNotice struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Text    string `yaml:"text" json:"text"`
	URL     string `yaml:"url" json:"url"`
	Icon    string `yaml:"icon" json:"icon"`
	Color   string `yaml:"color" json:"color"`
	BgColor string `yaml:"bg_color" json:"bg_color"`
}

// ConfigTemplate 单个配置类型的模板
type ConfigTemplate struct {
	Title string   `yaml:"title" json:"title"`
	Steps []string `yaml:"steps" json:"steps"`
}

// ConfigTemplates 配置生成器模板
type ConfigTemplates struct {
	EmptyHint  string         `yaml:"empty_hint" json:"empty_hint"`
	Docker     ConfigTemplate `yaml:"docker" json:"docker"`
	Podman     ConfigTemplate `yaml:"podman" json:"podman"`
	Containerd ConfigTemplate `yaml:"containerd" json:"containerd"`
	Nerdctl    ConfigTemplate `yaml:"nerdctl" json:"nerdctl"`
}

// DonateItem 捐赠方式
type DonateItem struct {
	Name   string `yaml:"name" json:"name"`
	Icon   string `yaml:"icon" json:"icon"`
	QRCode string `yaml:"qrcode" json:"qrcode"`
	Color  string `yaml:"color" json:"color"`
}

// Donate 捐赠配置
type Donate struct {
	Enabled     bool         `yaml:"enabled" json:"enabled"`
	Title       string       `yaml:"title" json:"title"`
	Description string       `yaml:"description" json:"description"`
	Items       []DonateItem `yaml:"items" json:"items"`
}

// AdItem 广告项
type AdItem struct {
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
	URL         string `yaml:"url" json:"url"`
	Image       string `yaml:"image" json:"image"`
	Color       string `yaml:"color" json:"color"`
}

// Ads 广告配置
type Ads struct {
	Enabled bool     `yaml:"enabled" json:"enabled"`
	Items   []AdItem `yaml:"items" json:"items"`
}

// AboutLink 关于页面链接
type AboutLink struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
	Icon string `yaml:"icon" json:"icon"`
}

// Disclaimer 免责声明配置
type Disclaimer struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	Title       string `yaml:"title" json:"title"`
	Content     string `yaml:"content" json:"content"`
	AIGenerated bool   `yaml:"ai_generated" json:"ai_generated"`
	AIStatement string `yaml:"ai_statement" json:"ai_statement"`
}

// About 关于页面配置
type About struct {
	Enabled        bool        `yaml:"enabled" json:"enabled"`
	Title          string      `yaml:"title" json:"title"`
	Description    string      `yaml:"description" json:"description"`
	CustomHTMLFile string      `yaml:"custom_html_file" json:"custom_html_file"`
	Donate         Donate      `yaml:"donate" json:"donate"`
	Ads            Ads         `yaml:"ads" json:"ads"`
	Links          []AboutLink `yaml:"links" json:"links"`
	Disclaimer     Disclaimer  `yaml:"disclaimer" json:"disclaimer"`
}

// Group 监控分组
type Group struct {
	ID          string   `yaml:"id" json:"id"`
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	OfficialURL string   `yaml:"official_url" json:"official_url"`
	Targets     []Target `yaml:"targets" json:"targets"`
}

// Config 配置结构
type Config struct {
	Server struct {
		Listen                string              `yaml:"listen"`
		Debug                 bool                `yaml:"debug"`                 // 调试模式：false=只显示错误/启动信息，true=显示所有连接日志
		RefreshInterval       time.Duration       `yaml:"refresh_interval"`
		SlowThreshold         int64               `yaml:"slow_threshold"`
		Proxy                 string              `yaml:"proxy"`
		UserAgent             string              `yaml:"user_agent"`
		APIToken              string              `yaml:"api_token"`               // API访问令牌
		RecommendedMirrors    map[string][]string `yaml:"recommended_mirrors"`     // 分组ID -> 推荐镜像URL列表
		RecommendedOnlyOnline bool                `yaml:"recommended_only_online"` // 推荐配置是否只选择在线镜像
		Site                  Site                `yaml:"site"`
		SiteNotice            SiteNotice          `yaml:"site_notice"`
		Footer                Footer              `yaml:"footer"`
		TagColors             []TagColor          `yaml:"tag_colors"`
		ConfigTemplates       ConfigTemplates     `yaml:"config_templates"`
		About                 About               `yaml:"about"`
		TLS                   struct {
			Enabled  bool   `yaml:"enabled"`
			CertFile string `yaml:"cert_file"`
			KeyFile  string `yaml:"key_file"`
		} `yaml:"tls"`
	} `yaml:"server"`
	Groups  []Group  `yaml:"groups"`
	Targets []Target `yaml:"targets"` // 向后兼容
}

// Target 监控目标
type Target struct {
	Name    string        `yaml:"name"`
	URL     string        `yaml:"url"`
	Method  string        `yaml:"method"`
	Timeout time.Duration `yaml:"timeout"`
	Tags    []string      `yaml:"tags"`
}

// Result 探测结果
type Result struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Status    string    `json:"status"`
	LatencyMs int64     `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
	Tags      []string  `json:"tags"`
}

// GroupStatus 分组状态
type GroupStatus struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	OfficialURL string    `json:"official_url"`
	Results     []*Result `json:"results"`
	Total       int       `json:"total"`
	Healthy     int       `json:"healthy"`
	Slow        int       `json:"slow"`
	Abnormal    int       `json:"abnormal"` // timeout + error
}

// StatusUpdate WebSocket推送的状态更新
type StatusUpdate struct {
	Groups    []GroupStatus `json:"groups"`
	CheckedAt string        `json:"checked_at"`
}

// Hub WebSocket连接管理
type Hub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan *StatusUpdate
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mu         sync.RWMutex
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan *StatusUpdate),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case conn := <-h.register:
			h.mu.Lock()
			h.clients[conn] = true
			clientCount := len(h.clients)
			h.mu.Unlock()
			logDebug("WebSocket客户端连接, 当前连接数: %d", clientCount)

		case conn := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
			clientCount := len(h.clients)
			h.mu.Unlock()
			logDebug("WebSocket客户端断开, 当前连接数: %d", clientCount)

		case update := <-h.broadcast:
			// 收集失败的连接，避免在遍历时修改map
			var failedConns []*websocket.Conn
			h.mu.RLock()
			for conn := range h.clients {
				// 设置写入超时，防止慢客户端阻塞 Hub
				conn.SetWriteDeadline(time.Now().Add(writeWait))
				err := conn.WriteJSON(update)
				if err != nil {
					failedConns = append(failedConns, conn)
				}
			}
			h.mu.RUnlock()

			// 统一处理失败的连接
			for _, conn := range failedConns {
				h.mu.Lock()
				if _, ok := h.clients[conn]; ok {
					delete(h.clients, conn)
					conn.Close()
				}
				h.mu.Unlock()
			}
		}
	}
}

func (h *Hub) Broadcast(update *StatusUpdate) {
	select {
	case h.broadcast <- update:
	default:
	}
}

// Monitor 监控器
type Monitor struct {
	config     *Config
	configPath string
	results    map[string]map[string]*Result // groupID -> targetName -> Result
	mu         sync.RWMutex
	hub        *Hub
	httpClient *http.Client
}

func NewMonitor(config *Config, configPath string, hub *Hub) *Monitor {
	results := make(map[string]map[string]*Result)
	for _, group := range config.Groups {
		results[group.ID] = make(map[string]*Result)
	}

	// 创建HTTP客户端（支持HTTP/HTTPS/SOCKS代理）
	transport := createProxyTransport(config.Server.Proxy)

	return &Monitor{
		config:     config,
		configPath: configPath,
		results:    results,
		hub:        hub,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
	}
}

// createProxyTransport 创建支持代理的Transport
func createProxyTransport(proxyAddr string) *http.Transport {
	transport := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if proxyAddr == "" {
		return transport
	}

	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		log.Printf("代理地址无效: %v", err)
		return transport
	}

	switch proxyURL.Scheme {
	case "socks5", "socks5h":
		// SOCKS5代理
		auth := &proxy.Auth{}
		if proxyURL.User != nil {
			auth.User = proxyURL.User.Username()
			auth.Password, _ = proxyURL.User.Password()
		} else {
			auth = nil
		}

		dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, proxy.Direct)
		if err != nil {
			log.Printf("创建SOCKS5代理失败: %v", err)
			return transport
		}

		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
		log.Printf("使用SOCKS5代理: %s", proxyURL.Host)

	case "http", "https":
		// HTTP/HTTPS代理
		transport.Proxy = http.ProxyURL(proxyURL)
		log.Printf("使用HTTP代理: %s", proxyAddr)

	default:
		log.Printf("不支持的代理协议: %s", proxyURL.Scheme)
	}

	return transport
}

// ReloadConfig 热加载配置文件
func (m *Monitor) ReloadConfig() error {
	newConfig, err := LoadAndValidateConfig(m.configPath)
	if err != nil {
		return fmt.Errorf("配置加载失败: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查代理配置是否改变
	if m.config.Server.Proxy != newConfig.Server.Proxy {
		oldProxy := m.config.Server.Proxy
		newTransport := createProxyTransport(newConfig.Server.Proxy)

		// 安全地替换transport
		m.httpClient.Transport = newTransport

		// 记录变化
		if newConfig.Server.Proxy == "" {
			log.Printf("代理已禁用 (原代理: %s)", oldProxy)
		} else {
			log.Printf("代理已更新: %s (原代理: %s)", newConfig.Server.Proxy, oldProxy)
		}
	}

	// 更新配置
	m.config = newConfig

	// 重建结果存储（保留已有结果）
	newResults := make(map[string]map[string]*Result)
	for _, group := range newConfig.Groups {
		newResults[group.ID] = make(map[string]*Result)
		// 尝试保留旧结果
		if oldGroupResults, ok := m.results[group.ID]; ok {
			for _, target := range group.Targets {
				if oldResult, ok := oldGroupResults[target.Name]; ok {
					newResults[group.ID][target.Name] = oldResult
				}
			}
		}
	}
	m.results = newResults

	log.Printf("配置热加载成功: %d 个分组, 共 %d 个监控目标", len(newConfig.Groups), m.getTotalTargets())
	return nil
}

func (m *Monitor) getTotalTargets() int {
	total := 0
	for _, group := range m.config.Groups {
		total += len(group.Targets)
	}
	return total
}

// GetConfig 获取当前配置（用于模板渲染）
func (m *Monitor) GetConfig() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

func (m *Monitor) Probe(target Target) *Result {
	result := &Result{
		Name:      target.Name,
		URL:       strings.TrimSuffix(target.URL, "/v2/"),
		CheckedAt: time.Now(),
		Tags:      target.Tags,
	}

	timeout := target.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	method := target.Method
	if method == "" {
		method = "HEAD"
	}

	req, err := http.NewRequestWithContext(ctx, method, target.URL, nil)
	if err != nil {
		result.Status = "error"
		result.LatencyMs = -1
		return result
	}

	// 使用读锁保护对共享资源的读取
	m.mu.RLock()
	userAgent := m.config.Server.UserAgent
	transport := m.httpClient.Transport
	m.mu.RUnlock()

	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	}
	req.Header.Set("User-Agent", userAgent)

	start := time.Now()

	// 创建带重定向控制的客户端（使用共享的Transport支持代理）
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		result.Status = "error"
		result.LatencyMs = -1
		return result
	}
	defer resp.Body.Close()

	result.LatencyMs = latency.Milliseconds()

	// 状态码 200、401、404 都视为服务可达
	isReachable := resp.StatusCode == 200 || resp.StatusCode == 401 || resp.StatusCode == 404 ||
		(resp.StatusCode >= 200 && resp.StatusCode < 400)

	// 获取配置的阈值（需注意并发读写安全，建议加锁或原子读取，或者简单点直接读）
	// 由于 m.config 在 ReloadConfig 时会被替换，这里最安全的做法是加读锁
	m.mu.RLock()
	slowThreshold := m.config.Server.SlowThreshold
	m.mu.RUnlock()
	
	if slowThreshold <= 0 {
		slowThreshold = 3000 // 默认值
	}

	if isReachable {
		// 使用该目标配置的超时时间作为判定基准 (target.Timeout 是 time.Duration)
		timeoutMs := target.Timeout.Milliseconds()
		// 如果配置为0(默认)，则设为5000作为基准
		if timeoutMs == 0 {
			timeoutMs = 5000
		}

		if result.LatencyMs > timeoutMs {
			result.Status = "timeout"
		} else if result.LatencyMs > slowThreshold {
			result.Status = "slow"
		} else {
			result.Status = "healthy"
		}
	} else {
		result.Status = "error"
	}

	return result
}

func (m *Monitor) ProbeAll() {
	var wg sync.WaitGroup

	for _, group := range m.config.Groups {
		for _, target := range group.Targets {
			wg.Add(1)
			go func(g Group, t Target) {
				defer wg.Done()

				time.Sleep(time.Duration(rand.Intn(2000)) * time.Millisecond)

				result := m.Probe(t)

				m.mu.Lock()
				if m.results[g.ID] == nil {
					m.results[g.ID] = make(map[string]*Result)
				}
				m.results[g.ID][t.Name] = result
				m.mu.Unlock()
			}(group, target)
		}
	}

	wg.Wait()

	m.broadcastStatus()
}

func (m *Monitor) broadcastStatus() {
	groupStatuses := m.GetGroupStatuses()

	update := &StatusUpdate{
		Groups:    groupStatuses,
		CheckedAt: time.Now().Format("2006-01-02 15:04:05"),
	}

	m.hub.Broadcast(update)
}

func (m *Monitor) GetGroupStatuses() []GroupStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var groupStatuses []GroupStatus
	for _, group := range m.config.Groups {
		results := make([]*Result, 0, len(group.Targets))
		healthyCount, slowCount, abnormalCount := 0, 0, 0

		for _, target := range group.Targets {
			if r, ok := m.results[group.ID][target.Name]; ok {
				results = append(results, r)
				switch r.Status {
				case "healthy":
					healthyCount++
				case "slow":
					slowCount++
				default: // timeout, error
					abnormalCount++
				}
			}
		}

		groupStatuses = append(groupStatuses, GroupStatus{
			ID:          group.ID,
			Name:        group.Name,
			Description: group.Description,
			OfficialURL: group.OfficialURL,
			Results:     results,
			Total:       len(results),
			Healthy:     healthyCount,
			Slow:        slowCount,
			Abnormal:    abnormalCount,
		})
	}

	return groupStatuses
}

func (m *Monitor) Start(ctx context.Context) {
	logDebug("执行初始探测...")
	m.ProbeAll()

	rand.Seed(time.Now().UnixNano())

	for {
		baseInterval := m.config.Server.RefreshInterval
		if baseInterval <= 0 {
			baseInterval = 120 * time.Second
		}

		jitterMax := int64(baseInterval) / 5 // 20%
		if jitterMax <= 0 {
			jitterMax = 1000
		}
		randomDelay := time.Duration(rand.Int63n(jitterMax))
		
		nextRunTime := baseInterval + randomDelay

		timer := time.NewTimer(nextRunTime)

		logDebug("下次探测将在 %v 后执行 (含随机抖动)...", nextRunTime.Round(time.Millisecond))

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			logDebug("执行定时探测...")
			m.ProbeAll()
		}
	}
}

func (m *Monitor) GetResults() []*Result {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 返回第一个分组的结果（向后兼容）
	if len(m.config.Groups) == 0 {
		return nil
	}

	group := m.config.Groups[0]
	results := make([]*Result, 0, len(group.Targets))
	for _, target := range group.Targets {
		if r, ok := m.results[group.ID][target.Name]; ok {
			results = append(results, r)
		}
	}
	return results
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	if config.Server.Listen == "" {
		config.Server.Listen = ":9080"
	}
	if config.Server.RefreshInterval == 0 {
		config.Server.RefreshInterval = 30 * time.Second
	}
	if config.Server.SlowThreshold == 0 {
		config.Server.SlowThreshold = 3000
	}

	// 向后兼容：如果没有groups但有targets，则创建默认分组
	if len(config.Groups) == 0 && len(config.Targets) > 0 {
		config.Groups = []Group{
			{
				ID:          "dockerhub",
				Name:        "Docker Hub",
				Description: "Docker官方镜像仓库",
				OfficialURL: "https://registry-1.docker.io",
				Targets:     config.Targets,
			},
		}
	}

	// 为所有分组的目标设置默认值
	for i := range config.Groups {
		for j := range config.Groups[i].Targets {
			if config.Groups[i].Targets[j].Timeout == 0 {
				config.Groups[i].Targets[j].Timeout = 5 * time.Second
			}
			if config.Groups[i].Targets[j].Method == "" {
				config.Groups[i].Targets[j].Method = "HEAD"
			}
		}
	}

	return &config, nil
}

// ValidateConfig 校验配置文件
func ValidateConfig(config *Config) error {
	// 校验服务器配置
	if config.Server.RefreshInterval < time.Second {
		return fmt.Errorf("refresh_interval 不能小于 1s")
	}
	if config.Server.SlowThreshold < 0 {
		return fmt.Errorf("slow_threshold 不能为负数")
	}

	// 校验分组配置
	if len(config.Groups) == 0 {
		return fmt.Errorf("至少需要配置一个监控分组")
	}

	groupIDs := make(map[string]bool)
	for i, group := range config.Groups {
		// 校验分组ID
		if group.ID == "" {
			return fmt.Errorf("分组 #%d: id 不能为空", i+1)
		}
		if groupIDs[group.ID] {
			return fmt.Errorf("分组 ID '%s' 重复", group.ID)
		}
		groupIDs[group.ID] = true

		// 校验分组名称
		if group.Name == "" {
			return fmt.Errorf("分组 '%s': name 不能为空", group.ID)
		}

		// 校验目标配置
		if len(group.Targets) == 0 {
			return fmt.Errorf("分组 '%s': 至少需要配置一个监控目标", group.ID)
		}

		targetNames := make(map[string]bool)
		for j, target := range group.Targets {
			if target.Name == "" {
				return fmt.Errorf("分组 '%s' 目标 #%d: name 不能为空", group.ID, j+1)
			}
			if targetNames[target.Name] {
				return fmt.Errorf("分组 '%s' 目标名称 '%s' 重复", group.ID, target.Name)
			}
			targetNames[target.Name] = true

			if target.URL == "" {
				return fmt.Errorf("分组 '%s' 目标 '%s': url 不能为空", group.ID, target.Name)
			}

			// 校验URL格式
			if _, err := url.Parse(target.URL); err != nil {
				return fmt.Errorf("分组 '%s' 目标 '%s': url 格式无效: %v", group.ID, target.Name, err)
			}

			if target.Timeout < 0 {
				return fmt.Errorf("分组 '%s' 目标 '%s': timeout 不能为负数", group.ID, target.Name)
			}
		}
	}

	// 校验标签颜色配置
	for i, tc := range config.Server.TagColors {
		if tc.Keyword == "" {
			return fmt.Errorf("tag_colors #%d: keyword 不能为空", i+1)
		}
		if tc.Color == "" {
			return fmt.Errorf("tag_colors '%s': color 不能为空", tc.Keyword)
		}
	}

	return nil
}

// fillDefaultTemplates 填充配置模板默认值
func (c *Config) fillDefaultTemplates() {
	if c.Server.ConfigTemplates.EmptyHint == "" {
		c.Server.ConfigTemplates.EmptyHint = "// 请先选择镜像源"
	}
	if c.Server.ConfigTemplates.Docker.Title == "" {
		c.Server.ConfigTemplates.Docker.Title = "Docker 镜像加速配置"
	}
	if len(c.Server.ConfigTemplates.Docker.Steps) == 0 {
		c.Server.ConfigTemplates.Docker.Steps = []string{
			"编辑配置文件: sudo vim /etc/docker/daemon.json",
			"重启 Docker: sudo systemctl restart docker",
		}
	}
	if c.Server.ConfigTemplates.Podman.Title == "" {
		c.Server.ConfigTemplates.Podman.Title = "Podman 配置"
	}
	if len(c.Server.ConfigTemplates.Podman.Steps) == 0 {
		c.Server.ConfigTemplates.Podman.Steps = []string{
			"编辑配置文件: sudo vim /etc/containers/registries.conf",
			"重启 Podman: sudo systemctl restart podman",
		}
	}
	if c.Server.ConfigTemplates.Containerd.Title == "" {
		c.Server.ConfigTemplates.Containerd.Title = "Containerd 配置"
	}
	if len(c.Server.ConfigTemplates.Containerd.Steps) == 0 {
		c.Server.ConfigTemplates.Containerd.Steps = []string{
			"编辑配置文件: sudo vim /etc/containerd/config.toml",
			"重启 Containerd: sudo systemctl restart containerd",
		}
	}
	if c.Server.ConfigTemplates.Nerdctl.Title == "" {
		c.Server.ConfigTemplates.Nerdctl.Title = "Nerdctl/Containerd hosts.toml"
	}
	if len(c.Server.ConfigTemplates.Nerdctl.Steps) == 0 {
		c.Server.ConfigTemplates.Nerdctl.Steps = []string{
			"创建目录: sudo mkdir -p /etc/containerd/certs.d/{registry}",
			"编辑配置: sudo vim /etc/containerd/certs.d/{registry}/hosts.toml",
		}
	}
}

// fillDefaultAbout 填充关于页面默认值
func (c *Config) fillDefaultAbout() {
	if c.Server.About.Title == "" {
		c.Server.About.Title = "关于本项目"
	}
	if c.Server.About.Donate.Title == "" {
		c.Server.About.Donate.Title = "支持本项目"
	}
}

// fillDefaultSiteNotice 填充顶部通知默认值
func (c *Config) fillDefaultSiteNotice() {
	if c.Server.SiteNotice.Enabled {
		if c.Server.SiteNotice.Color == "" {
			c.Server.SiteNotice.Color = "#ff6b6b"
		}
		if c.Server.SiteNotice.BgColor == "" {
			c.Server.SiteNotice.BgColor = "rgba(255,107,107,0.1)"
		}
		if c.Server.SiteNotice.Icon == "" {
			c.Server.SiteNotice.Icon = "fas fa-bullhorn"
		}
	}
}

// fillDefaultSite 填充站点配置默认值
func (c *Config) fillDefaultSite() {
	if c.Server.Site.Logo == "" {
		c.Server.Site.Logo = "容器镜像监控"
	}
	if c.Server.Site.LogoIcon == "" {
		// 修改默认值为完整的类名
		c.Server.Site.LogoIcon = "fab fa-docker"
	}
	if c.Server.Site.Title == "" {
		c.Server.Site.Title = "容器镜像加速器监控"
	}
	if c.Server.Site.Description == "" {
		c.Server.Site.Description = "实时监控多种容器镜像源的加速器状态，帮助开发者选择最佳的镜像源"
	}
	if c.Server.Site.BrowserTitle == "" {
		c.Server.Site.BrowserTitle = "容器镜像监控"
	}
}

// LoadAndValidateConfig 加载并校验配置
func LoadAndValidateConfig(path string) (*Config, error) {
	config, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}

	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("配置校验失败: %w", err)
	}

	// 填充配置模板默认值
	config.fillDefaultTemplates()

	// 填充关于页面默认值
	config.fillDefaultAbout()

	// 填充顶部通知默认值
	config.fillDefaultSiteNotice()

	// 填充站点配置默认值
	config.fillDefaultSite()

	return config, nil
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta name="description" content="{{.Site.Description}}">
    <title>{{.Site.BrowserTitle}}</title>
    {{if .Site.Favicon}}<link rel="icon" href="{{.Site.Favicon}}" type="image/x-icon">{{end}}
    
    <link rel="stylesheet" href="https://cdn.bootcdn.net/ajax/libs/font-awesome/5.15.4/css/all.min.css" onerror="loadFallbackCSS()">
    
    <script>
        function loadFallbackCSS() {
            console.warn('BootCDN failed, switching to cdnjs fallback...');
            var link = document.createElement('link');
            link.rel = 'stylesheet';
            // 备用源：使用 cdnjs (Cloudflare)
            link.href = 'https://cdnjs.cloudflare.com/ajax/libs/font-awesome/5.15.4/css/all.min.css';
            document.head.appendChild(link);
        }
    </script>

    <style>
        /* 默认主题 - 紫蓝渐变 */
        :root {
            --bg-primary: #f0f5ff;
            --bg-secondary: #ffffff;
            --bg-tertiary: #f5f8ff;
            --text-primary: #1e293b;
            --text-secondary: #64748b;
            --text-tertiary: #94a3b8;
            --border-color: #e0e7ff;
            --primary: #4f6ef7;
            --primary-light: #6366f1;
            --primary-dark: #3b4fd9;
            --gradient-start: #5a7bff;
            --gradient-end: #8b5cf6;
            --success: #10b981;
            --warning: #f59e0b;
            --danger: #ef4444;
            --card-shadow: 0 4px 20px rgba(99, 102, 241, 0.08);
            --card-shadow-hover: 0 8px 30px rgba(99, 102, 241, 0.15);
        }
        .dark {
            --bg-primary: #0f172a;
            --bg-secondary: #1e293b;
            --bg-tertiary: #334155;
            --text-primary: #f1f5f9;
            --text-secondary: #94a3b8;
            --text-tertiary: #64748b;
            --border-color: #334155;
            --card-shadow: 0 4px 20px rgba(0,0,0,0.3);
            --card-shadow-hover: 0 8px 30px rgba(0,0,0,0.4);
        }
        
        /* 清新蓝主题 */
        .theme-fresh-blue {
            --bg-primary: #e8f4fc;
            --bg-secondary: #ffffff;
            --bg-tertiary: #f0f7fc;
            --text-primary: #1a365d;
            --text-secondary: #4a6fa5;
            --text-tertiary: #7b9cc4;
            --border-color: #bee3f8;
            --primary: #3182ce;
            --primary-light: #4299e1;
            --primary-dark: #2b6cb0;
            --gradient-start: #4facfe;
            --gradient-end: #00f2fe;
            --success: #38a169;
            --warning: #dd6b20;
            --danger: #e53e3e;
            --card-shadow: 0 4px 20px rgba(49, 130, 206, 0.1);
            --card-shadow-hover: 0 8px 30px rgba(49, 130, 206, 0.18);
        }
        .theme-fresh-blue.dark {
            --bg-primary: #0d1b2a;
            --bg-secondary: #1b2838;
            --bg-tertiary: #2d3e50;
            --text-primary: #e2e8f0;
            --text-secondary: #a0aec0;
            --text-tertiary: #718096;
            --border-color: #2d3e50;
            --card-shadow: 0 4px 20px rgba(0,0,0,0.35);
            --card-shadow-hover: 0 8px 30px rgba(0,0,0,0.45);
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Inter', sans-serif;
            background: var(--bg-primary);
            color: var(--text-primary);
            min-height: 100vh;
            transition: background-color 0.3s, color 0.3s;
        }
        .container { max-width: 1200px; margin: 0 auto; padding: 0 20px; }
        
        header {
            background: var(--bg-secondary);
            border-bottom: 1px solid var(--border-color);
            position: sticky;
            top: 0;
            z-index: 100;
            transition: background-color 0.3s;
            box-shadow: 0 2px 10px rgba(99, 102, 241, 0.05);
        }
        .header-content {
            display: flex;
            justify-content: space-between;
            align-items: center;
            flex-wrap: wrap;
            gap: 10px;
            min-height: 70px;
            padding: 10px 0;
        }
        .header-row {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        @media (max-width: 768px) {
            .header-content { justify-content: center; }
            .top-notice { 
                order: 3;
            }
        }
        .logo {
            display: flex;
            align-items: center;
            gap: 12px;
            font-size: 1.3rem;
            font-weight: 700;
        }
        .logo i { 
            background: linear-gradient(135deg, var(--gradient-start) 0%, var(--gradient-end) 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
            font-size: 1.75rem; 
        }
        .top-notice {
            display: flex;
            align-items: center;
            gap: 6px;
            padding: 8px 16px;
            border-radius: 20px;
            font-size: 0.85rem;
            font-weight: 500;
            text-decoration: none;
            transition: all 0.3s;
            border: 1px solid currentColor;
            opacity: 0.9;
            white-space: nowrap;
        }
        .top-notice:hover {
            opacity: 1;
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(0,0,0,0.15);
        }
        .top-notice i { font-size: 0.9rem; }
        .header-actions { display: flex; align-items: center; gap: 12px; }
        .ws-status {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 0.8rem;
            color: var(--text-secondary);
            padding: 6px 14px;
            background: var(--bg-tertiary);
            border-radius: 20px;
            border: 1px solid var(--border-color);
        }
        .ws-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--danger);
            transition: background 0.3s;
        }
        .ws-dot.connected { 
            background: var(--success); 
            box-shadow: 0 0 8px var(--success);
        }
        #theme-toggle {
            width: 42px;
            height: 42px;
            border-radius: 12px;
            border: none;
            background: var(--bg-tertiary);
            color: var(--text-secondary);
            cursor: pointer;
            transition: all 0.3s;
            display: flex;
            align-items: center;
            justify-content: center;
            border: 1px solid var(--border-color);
        }
        #theme-toggle:hover { 
            background: linear-gradient(135deg, var(--gradient-start) 0%, var(--gradient-end) 100%);
            color: #fff;
            border-color: transparent;
        }
        
        /* 主题选择器样式 */
        .theme-selector {
            position: relative;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .theme-selector-btn {
            display: flex;
            align-items: center;
            gap: 6px;
            padding: 8px 14px;
            border-radius: 10px;
            border: 1px solid var(--border-color);
            background: var(--bg-tertiary);
            color: var(--text-secondary);
            font-size: 0.8rem;
            cursor: pointer;
            transition: all 0.3s;
        }
        .theme-selector-btn:hover {
            border-color: var(--primary);
            color: var(--primary);
        }
        .theme-selector-btn i { font-size: 0.9rem; }
        .theme-dropdown {
            position: absolute;
            top: 100%;
            right: 0;
            margin-top: 8px;
            background: var(--bg-secondary);
            border: 1px solid var(--border-color);
            border-radius: 12px;
            box-shadow: var(--card-shadow-hover);
            z-index: 1000;
            min-width: 160px;
            display: none;
            overflow: hidden;
        }
        .theme-dropdown.show { display: block; }
        .theme-option {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 12px 16px;
            cursor: pointer;
            transition: all 0.2s;
            font-size: 0.85rem;
            color: var(--text-primary);
        }
        .theme-option:hover { background: var(--bg-tertiary); }
        .theme-option.active { 
            background: linear-gradient(135deg, rgba(var(--gradient-start-rgb, 90,123,255), 0.1) 0%, rgba(var(--gradient-end-rgb, 139,92,246), 0.1) 100%);
            color: var(--primary);
        }
        .theme-option .theme-preview {
            width: 18px;
            height: 18px;
            border-radius: 50%;
            border: 2px solid var(--border-color);
        }
        .theme-option .theme-preview.preview-default {
            background: linear-gradient(135deg, #5a7bff 0%, #8b5cf6 100%);
        }
        .theme-option .theme-preview.preview-fresh-blue {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
        }
        
        main { padding: 28px 0 48px; }
        .page-title { 
            margin-bottom: 10px; 
            font-size: 1.85rem; 
            font-weight: 700;
            background: linear-gradient(135deg, var(--gradient-start) 0%, var(--gradient-end) 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        .page-desc { color: var(--text-secondary); margin-bottom: 28px; font-size: 1rem; }
        
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 16px;
            margin-bottom: 24px;
        }
        @media (max-width: 768px) {
            .stats-grid { grid-template-columns: repeat(2, 1fr); }
        }
        .stat-card {
            background: var(--bg-secondary);
            border-radius: 16px;
            padding: 24px;
            box-shadow: var(--card-shadow);
            transition: transform 0.3s, box-shadow 0.3s, background 0.3s;
            position: relative;
            overflow: hidden;
        }
        .stat-card:hover { transform: translateY(-4px); box-shadow: var(--card-shadow-hover); }
        .stat-card.gradient-card {
            background: linear-gradient(135deg, var(--gradient-start) 0%, var(--gradient-end) 100%);
            color: #fff;
        }
        .dark .stat-card.gradient-card {
            background: linear-gradient(135deg, rgba(90,123,255,0.25) 0%, rgba(139,92,246,0.25) 100%);
            border: 1px solid rgba(139,92,246,0.3);
        }
        .dark .stat-card.gradient-card .stat-value { color: #c4b5fd !important; }
        .stat-card.gradient-card .stat-label { color: rgba(255,255,255,0.85); }
        .dark .stat-card.gradient-card .stat-label { color: #a5b4fc; }
        .stat-card.gradient-card .stat-value { color: #fff !important; }
        .stat-card.gradient-card .stat-icon { background: rgba(255,255,255,0.2); color: #fff; }
        .dark .stat-card.gradient-card .stat-icon { background: rgba(139,92,246,0.3); color: #c4b5fd; }
        
        .stat-card.gradient-success {
            background: linear-gradient(135deg, #10b981 0%, #34d399 100%);
            color: #fff;
        }
        .stat-card.gradient-success .stat-label { color: rgba(255,255,255,0.85); }
        .stat-card.gradient-success .stat-value { color: #fff !important; }
        .stat-card.gradient-success .stat-icon { background: rgba(255,255,255,0.2); color: #fff; }
        .dark .stat-card.gradient-success {
            background: linear-gradient(135deg, rgba(16,185,129,0.25) 0%, rgba(52,211,153,0.25) 100%);
            border: 1px solid rgba(16,185,129,0.3);
        }
        .dark .stat-card.gradient-success .stat-label { color: #6ee7b7; }
        .dark .stat-card.gradient-success .stat-value { color: #a7f3d0 !important; }
        .dark .stat-card.gradient-success .stat-icon { background: rgba(16,185,129,0.3); color: #6ee7b7; }
        
        .stat-card.gradient-warning {
            background: linear-gradient(135deg, #f59e0b 0%, #fbbf24 100%);
            color: #fff;
        }
        .stat-card.gradient-warning .stat-label { color: rgba(255,255,255,0.85); }
        .stat-card.gradient-warning .stat-value { color: #fff !important; }
        .stat-card.gradient-warning .stat-icon { background: rgba(255,255,255,0.2); color: #fff; }
        .dark .stat-card.gradient-warning {
            background: linear-gradient(135deg, rgba(245,158,11,0.25) 0%, rgba(251,191,36,0.25) 100%);
            border: 1px solid rgba(245,158,11,0.3);
        }
        .dark .stat-card.gradient-warning .stat-label { color: #fcd34d; }
        .dark .stat-card.gradient-warning .stat-value { color: #fde68a !important; }
        .dark .stat-card.gradient-warning .stat-icon { background: rgba(245,158,11,0.3); color: #fcd34d; }
        
        .stat-card.gradient-danger {
            background: linear-gradient(135deg, #ef4444 0%, #f87171 100%);
            color: #fff;
        }
        .stat-card.gradient-danger .stat-label { color: rgba(255,255,255,0.85); }
        .stat-card.gradient-danger .stat-value { color: #fff !important; }
        .stat-card.gradient-danger .stat-icon { background: rgba(255,255,255,0.2); color: #fff; }
        .dark .stat-card.gradient-danger {
            background: linear-gradient(135deg, rgba(239,68,68,0.25) 0%, rgba(248,113,113,0.25) 100%);
            border: 1px solid rgba(239,68,68,0.3);
        }
        .dark .stat-card.gradient-danger .stat-label { color: #fca5a5; }
        .dark .stat-card.gradient-danger .stat-value { color: #fecaca !important; }
        .dark .stat-card.gradient-danger .stat-icon { background: rgba(239,68,68,0.3); color: #fca5a5; }
        
        /* 清新蓝主题特定样式 */
        .theme-fresh-blue .stat-card.gradient-card {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
        }
        .theme-fresh-blue.dark .stat-card.gradient-card {
            background: linear-gradient(135deg, rgba(79,172,254,0.25) 0%, rgba(0,242,254,0.25) 100%);
            border: 1px solid rgba(79,172,254,0.3);
        }
        .theme-fresh-blue.dark .stat-card.gradient-card .stat-value { color: #7dd3fc !important; }
        .theme-fresh-blue.dark .stat-card.gradient-card .stat-label { color: #bae6fd; }
        .theme-fresh-blue.dark .stat-card.gradient-card .stat-icon { background: rgba(79,172,254,0.3); color: #7dd3fc; }
        
        .theme-fresh-blue .stat-card.gradient-success {
            background: linear-gradient(135deg, #38a169 0%, #68d391 100%);
        }
        .theme-fresh-blue.dark .stat-card.gradient-success {
            background: linear-gradient(135deg, rgba(56,161,105,0.25) 0%, rgba(104,211,145,0.25) 100%);
            border: 1px solid rgba(56,161,105,0.3);
        }
        
        .theme-fresh-blue .stat-card.gradient-warning {
            background: linear-gradient(135deg, #dd6b20 0%, #f6ad55 100%);
        }
        .theme-fresh-blue.dark .stat-card.gradient-warning {
            background: linear-gradient(135deg, rgba(221,107,32,0.25) 0%, rgba(246,173,85,0.25) 100%);
            border: 1px solid rgba(221,107,32,0.3);
        }
        
        .theme-fresh-blue .stat-card.gradient-danger {
            background: linear-gradient(135deg, #e53e3e 0%, #fc8181 100%);
        }
        .theme-fresh-blue.dark .stat-card.gradient-danger {
            background: linear-gradient(135deg, rgba(229,62,62,0.25) 0%, rgba(252,129,129,0.25) 100%);
            border: 1px solid rgba(229,62,62,0.3);
        }
        
        .theme-fresh-blue .tab-btn.active {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
            box-shadow: 0 4px 12px rgba(79,172,254,0.3);
        }
        .theme-fresh-blue.dark .tab-btn.active {
            background: linear-gradient(135deg, rgba(79,172,254,0.3) 0%, rgba(0,242,254,0.3) 100%);
            border-color: rgba(79,172,254,0.5);
        }
        
        .theme-fresh-blue .page-title {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .theme-fresh-blue .logo i {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .theme-fresh-blue #theme-toggle:hover {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
        }
        
        .theme-fresh-blue .btn-primary {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
            box-shadow: 0 2px 8px rgba(79,172,254,0.3);
        }
        .theme-fresh-blue .btn-primary:hover {
            box-shadow: 0 4px 12px rgba(79,172,254,0.4);
        }
        .theme-fresh-blue.dark .btn-primary {
            background: rgba(79,172,254,0.2);
            color: #7dd3fc;
            box-shadow: none;
            border: 1px solid rgba(79,172,254,0.3);
        }
        .theme-fresh-blue.dark .btn-primary:hover {
            background: rgba(79,172,254,0.3);
            box-shadow: 0 0 12px rgba(79,172,254,0.2);
        }
        
        .theme-fresh-blue .mirror-option:hover {
            box-shadow: 0 2px 8px rgba(79,172,254,0.1);
        }
        .theme-fresh-blue .mirror-option.selected {
            background: linear-gradient(135deg, rgba(79,172,254,0.1) 0%, rgba(0,242,254,0.1) 100%);
        }
        .theme-fresh-blue.dark .mirror-option.selected {
            background: linear-gradient(135deg, rgba(79,172,254,0.15) 0%, rgba(0,242,254,0.15) 100%);
            border-color: rgba(79,172,254,0.5);
        }
        
        .theme-fresh-blue th {
            background: linear-gradient(135deg, rgba(79,172,254,0.08) 0%, rgba(0,242,254,0.08) 100%);
        }
        
        .theme-fresh-blue .site-url {
            background: linear-gradient(135deg, rgba(79,172,254,0.08) 0%, rgba(0,242,254,0.08) 100%);
        }
        .theme-fresh-blue .site-url:hover {
            background: linear-gradient(135deg, rgba(79,172,254,0.15) 0%, rgba(0,242,254,0.15) 100%);
        }
        
        .theme-fresh-blue .stat-icon.total {
            background: linear-gradient(135deg, rgba(79,172,254,0.15) 0%, rgba(0,242,254,0.15) 100%);
            color: var(--primary);
        }
        
        .theme-fresh-blue footer {
            background: linear-gradient(135deg, rgba(79,172,254,0.05) 0%, rgba(0,242,254,0.05) 100%);
        }
        
        .theme-fresh-blue footer a:hover {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .theme-fresh-blue .link-item:hover {
            background: linear-gradient(135deg, rgba(79,172,254,0.1) 0%, rgba(0,242,254,0.1) 100%);
        }
        
        .theme-fresh-blue .ai-generated-badge {
            background: linear-gradient(135deg, rgba(79,172,254,0.1) 0%, rgba(0,242,254,0.1) 100%);
            border: 1px solid rgba(79,172,254,0.2);
        }
        
        .theme-fresh-blue.dark .mirror-grid {
            background: rgba(27, 40, 56, 0.5);
            border-color: rgba(45, 62, 80, 0.5);
        }
        .theme-fresh-blue.dark .mirror-option {
            background: rgba(27, 40, 56, 0.8);
            border-color: rgba(45, 62, 80, 0.3);
        }
        .theme-fresh-blue.dark .mirror-option:hover {
            background: rgba(45, 62, 80, 0.8);
            box-shadow: 0 2px 8px rgba(79,172,254,0.15);
        }
        .stat-card-content { display: flex; justify-content: space-between; align-items: center; }
        .stat-label { color: var(--text-secondary); font-size: 0.875rem; font-weight: 500; }
        .stat-value { font-size: 2.25rem; font-weight: 700; margin-top: 8px; transition: all 0.3s; }
        .stat-icon {
            width: 56px;
            height: 56px;
            border-radius: 16px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.5rem;
        }
        .stat-icon.total { background: linear-gradient(135deg, rgba(90,123,255,0.15) 0%, rgba(139,92,246,0.15) 100%); color: var(--primary); }
        .stat-icon.online { background: rgba(16,185,129,0.12); color: var(--success); }
        .stat-icon.slow { background: rgba(245,158,11,0.12); color: var(--warning); }
        .stat-icon.offline { background: rgba(239,68,68,0.12); color: var(--danger); }
        
        .card {
            background: var(--bg-secondary);
            border-radius: 20px;
            padding: 28px;
            box-shadow: var(--card-shadow);
            margin-bottom: 24px;
            transition: background-color 0.3s, box-shadow 0.3s;
            border: 1px solid var(--border-color);
        }
        .card:hover { box-shadow: var(--card-shadow-hover); }
        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            flex-wrap: wrap;
            gap: 12px;
        }
        .card-title { font-size: 1.125rem; font-weight: 600; }
        .config-type-select {
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .config-type-select label { color: var(--text-secondary); font-size: 0.875rem; }
        .config-type-select select {
            background: var(--bg-tertiary);
            border: 1px solid var(--border-color);
            border-radius: 10px;
            padding: 10px 14px;
            color: var(--text-primary);
            font-size: 0.875rem;
            cursor: pointer;
            transition: all 0.2s;
        }
        .config-type-select select:hover {
            border-color: var(--primary);
        }
        .config-type-select select:focus {
            outline: none;
            border-color: var(--primary);
            box-shadow: 0 0 0 3px rgba(99,102,241,0.1);
        }
        
        .mirror-selection-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
            flex-wrap: wrap;
            gap: 8px;
        }
        .mirror-selection-title { font-size: 0.875rem; font-weight: 500; }
        .mirror-selection-actions { display: flex; gap: 8px; flex-wrap: wrap; }
        .btn-sm {
            padding: 8px 16px;
            border-radius: 10px;
            border: none;
            font-size: 0.8rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.3s;
            display: flex;
            align-items: center;
            gap: 6px;
        }
        .btn-success { 
            background: linear-gradient(135deg, #10b981 0%, #34d399 100%); 
            color: #fff; 
            box-shadow: 0 2px 8px rgba(16,185,129,0.3);
        }
        .btn-success:hover { 
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(16,185,129,0.4); 
        }
        .dark .btn-success {
            background: rgba(16,185,129,0.2);
            color: #34d399;
            box-shadow: none;
            border: 1px solid rgba(16,185,129,0.3);
        }
        .dark .btn-success:hover {
            background: rgba(16,185,129,0.3);
            box-shadow: 0 0 12px rgba(16,185,129,0.2);
        }
        .btn-primary { 
            background: linear-gradient(135deg, var(--gradient-start) 0%, var(--gradient-end) 100%); 
            color: #fff;
            box-shadow: 0 2px 8px rgba(99,102,241,0.3);
        }
        .btn-primary:hover { 
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(99,102,241,0.4); 
        }
        .dark .btn-primary {
            background: rgba(99,102,241,0.2);
            color: #a5b4fc;
            box-shadow: none;
            border: 1px solid rgba(99,102,241,0.3);
        }
        .dark .btn-primary:hover {
            background: rgba(99,102,241,0.3);
            box-shadow: 0 0 12px rgba(99,102,241,0.2);
        }
        .btn-secondary { 
            background: var(--bg-tertiary); 
            color: var(--text-secondary);
            border: 1px solid var(--border-color);
        }
        .btn-secondary:hover { background: var(--border-color); }
        
        .mirror-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
            gap: 10px;
            max-height: 260px;
            overflow-y: auto;
            padding: 16px;
            background: var(--bg-tertiary);
            border-radius: 16px;
            margin-bottom: 20px;
            border: 1px solid var(--border-color);
        }
        .dark .mirror-grid {
            background: rgba(30, 41, 59, 0.5);
            border-color: rgba(71, 85, 105, 0.5);
        }
        .mirror-option {
            display: flex;
            align-items: center;
            padding: 12px 14px;
            border-radius: 12px;
            cursor: pointer;
            transition: all 0.2s;
            background: var(--bg-secondary);
            border: 1px solid transparent;
        }
        .dark .mirror-option {
            background: rgba(30, 41, 59, 0.8);
            border-color: rgba(71, 85, 105, 0.3);
        }
        .mirror-option:hover { 
            border-color: var(--primary);
            box-shadow: 0 2px 8px rgba(99,102,241,0.1);
        }
        .dark .mirror-option:hover {
            background: rgba(51, 65, 85, 0.8);
            box-shadow: 0 2px 8px rgba(99,102,241,0.15);
        }
        .mirror-option.selected { 
            background: linear-gradient(135deg, rgba(90,123,255,0.1) 0%, rgba(139,92,246,0.1) 100%);
            border-color: var(--primary);
        }
        .dark .mirror-option.selected {
            background: linear-gradient(135deg, rgba(90,123,255,0.15) 0%, rgba(139,92,246,0.15) 100%);
            border-color: rgba(99,102,241,0.5);
        }
        .mirror-option input[type="checkbox"] { margin-right: 10px; accent-color: var(--primary); }
        .mirror-info { flex: 1; min-width: 0; }
        .mirror-name-row { display: flex; align-items: center; gap: 6px; }
        .mirror-status {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            flex-shrink: 0;
            transition: background-color 0.3s;
        }
        .mirror-status.healthy { background: var(--success); }
        .mirror-status.slow { background: var(--warning); }
        .mirror-status.timeout { background: #ff9800; }
        .mirror-status.error { background: var(--danger); }
        .mirror-name { font-size: 0.875rem; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .mirror-url { font-size: 0.75rem; color: var(--text-tertiary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .mirror-hint { font-size: 0.75rem; color: var(--text-tertiary); margin-top: 8px; }
        .mirror-hint i { margin-right: 4px; }
        
        .config-output-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 12px;
        }
        .code-block {
            background: linear-gradient(135deg, #1e1b4b 0%, #0f172a 100%);
            border-radius: 16px;
            padding: 20px;
            overflow-x: auto;
            position: relative;
            border: 1px solid #334155;
        }
        .dark .code-block {
            background: linear-gradient(135deg, #020617 0%, #0f172a 100%);
            border: 1px solid #475569;
        }
        .code-block pre { margin: 0; }
        .code-block code {
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            font-size: 0.8rem;
            color: #e2e8f0;
            white-space: pre;
            line-height: 1.6;
        }
        .code-block .comment { color: #94a3b8; font-style: italic; }
        .code-block .string { color: #86efac; }
        .code-block .key { color: #93c5fd; }
        .code-block .path { color: #fbbf24; }
        
        .table-container { 
            overflow-x: auto; 
            -webkit-overflow-scrolling: touch;
            scrollbar-width: thin;
        }
        .table-container::-webkit-scrollbar { height: 6px; }
        .table-container::-webkit-scrollbar-track { background: var(--bg-tertiary); border-radius: 3px; }
        .table-container::-webkit-scrollbar-thumb { background: var(--border-color); border-radius: 3px; }
        table { width: 100%; border-collapse: collapse; min-width: 800px; }
        th, td { padding: 16px; text-align: center; }
        th {
            background: linear-gradient(135deg, rgba(90,123,255,0.08) 0%, rgba(139,92,246,0.08) 100%);
            font-size: 0.75rem;
            font-weight: 600;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.05em;
        }
        th:first-child { border-radius: 12px 0 0 0; }
        th:last-child { border-radius: 0 12px 0 0; }
        tbody tr { border-bottom: 1px solid var(--border-color); transition: all 0.2s; }
        tbody tr:hover { background: var(--bg-tertiary); }
        td { font-size: 0.875rem; }
        
        .status-dot {
            width: 12px;
            height: 12px;
            border-radius: 50%;
            display: inline-block;
            transition: all 0.3s;
        }
        .status-dot.healthy { background: var(--success); box-shadow: 0 0 8px var(--success); animation: pulse-success 2s infinite; }
        .status-dot.slow { background: var(--warning); box-shadow: 0 0 8px var(--warning); animation: pulse-warning 2s infinite; }
        .status-dot.timeout { background: #ff9800; box-shadow: 0 0 8px #ff9800; animation: pulse-warning 2s infinite; }
        .status-dot.error { background: var(--danger); box-shadow: 0 0 8px var(--danger); animation: pulse-danger 2s infinite; }
        @keyframes pulse-success { 0%, 100% { box-shadow: 0 0 0 0 rgba(0,180,42,0.4); } 50% { box-shadow: 0 0 8px 4px rgba(0,180,42,0.2); } }
        @keyframes pulse-warning { 0%, 100% { box-shadow: 0 0 0 0 rgba(255,125,0,0.4); } 50% { box-shadow: 0 0 8px 4px rgba(255,125,0,0.2); } }
        @keyframes pulse-danger { 0%, 100% { box-shadow: 0 0 0 0 rgba(245,63,63,0.4); } 50% { box-shadow: 0 0 8px 4px rgba(245,63,63,0.2); } }
        
        .site-name { font-weight: 600; }
        .site-url {
            display: inline-block;
            background: linear-gradient(135deg, rgba(90,123,255,0.08) 0%, rgba(139,92,246,0.08) 100%);
            padding: 6px 12px;
            border-radius: 8px;
            font-family: monospace;
            font-size: 0.8rem;
            color: var(--text-secondary);
            cursor: pointer;
            transition: all 0.2s;
            border: 1px solid transparent;
        }
        .site-url:hover { 
            border-color: var(--primary);
            background: linear-gradient(135deg, rgba(90,123,255,0.15) 0%, rgba(139,92,246,0.15) 100%);
        }
        
        .tags { display: flex; flex-wrap: wrap; gap: 6px; justify-content: center; }
        .tag {
            display: inline-block;
            padding: 4px 10px;
            border-radius: 20px;
            font-size: 0.7rem;
            font-weight: 500;
        }
        .tag-cloudflare { background: linear-gradient(135deg, rgba(246,130,30,0.15) 0%, rgba(255,165,0,0.15) 100%); color: #f6821e; }
        .tag-nginx { background: linear-gradient(135deg, rgba(68,160,71,0.15) 0%, rgba(76,175,80,0.15) 100%); color: #44a047; }
        .tag-aliyun { background: linear-gradient(135deg, rgba(255,153,0,0.15) 0%, rgba(255,193,7,0.15) 100%); color: #ff9900; }
        .tag-tencent { background: linear-gradient(135deg, rgba(0,164,255,0.15) 0%, rgba(33,150,243,0.15) 100%); color: #00a4ff; }
        .tag-1panel { background: linear-gradient(135deg, rgba(0,94,235,0.15) 0%, rgba(63,81,181,0.15) 100%); color: #005eeb; }
        .tag-daocloud { background: linear-gradient(135deg, rgba(0,94,235,0.15) 0%, rgba(63,81,181,0.15) 100%); color: #005eeb; }
        .tag-warning { background: linear-gradient(135deg, rgba(255,87,34,0.15) 0%, rgba(244,67,54,0.15) 100%); color: #ff5722; }
        .tag-default { background: linear-gradient(135deg, rgba(120,120,120,0.15) 0%, rgba(158,158,158,0.15) 100%); color: #787878; }
        {{range .TagColors}}
        .tag-custom-{{.Keyword}} { background: {{if .BgColor}}{{.BgColor}}{{else}}rgba(100,100,100,0.15){{end}}; color: {{.Color}}; }
        {{end}}
        
        .check-time { font-size: 0.875rem; }
        
        .legend {
            display: flex;
            gap: 24px;
            margin-top: 16px;
            font-size: 0.75rem;
            color: var(--text-secondary);
        }
        .legend-item { display: flex; align-items: center; gap: 6px; }
        
        footer {
            background: linear-gradient(135deg, rgba(90,123,255,0.05) 0%, rgba(139,92,246,0.05) 100%);
            border-top: 1px solid var(--border-color);
            padding: 28px 0;
            text-align: center;
            font-size: 0.875rem;
            color: var(--text-secondary);
        }
        footer a { 
            color: var(--primary); 
            text-decoration: none; 
            transition: all 0.2s;
            font-weight: 500;
        }
        footer a:hover { 
            background: linear-gradient(135deg, var(--gradient-start) 0%, var(--gradient-end) 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        
        .update-flash {
            animation: flash 0.5s ease-out;
        }
        @keyframes flash {
            0% { background-color: rgba(22,93,255,0.2); }
            100% { background-color: transparent; }
        }
        
        /* Toast 提示样式 */
        .toast-container {
            position: fixed;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            z-index: 10000;
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 10px;
            pointer-events: none;
        }
        .toast {
            background: var(--card-bg);
            color: var(--text-primary);
            padding: 12px 24px;
            border-radius: 8px;
            box-shadow: 0 4px 12px rgba(0,0,0,0.15);
            border: 1px solid var(--border-color);
            display: flex;
            align-items: center;
            gap: 10px;
            animation: toastIn 0.3s ease-out;
            pointer-events: auto;
        }
        .toast.success { border-left: 4px solid var(--success); }
        .toast.success i { color: var(--success); }
        .toast.error { border-left: 4px solid var(--danger); }
        .toast.error i { color: var(--danger); }
        .toast.hiding {
            animation: toastOut 0.3s ease-out forwards;
        }
        @keyframes toastIn {
            from { opacity: 0; transform: scale(0.9); }
            to { opacity: 1; transform: scale(1); }
        }
        @keyframes toastOut {
            from { opacity: 1; transform: scale(1); }
            to { opacity: 0; transform: scale(0.9); }
        }
        
        ::-webkit-scrollbar { width: 6px; height: 6px; }
        ::-webkit-scrollbar-track { background: transparent; }
        ::-webkit-scrollbar-thumb { background: var(--border-color); border-radius: 3px; }
        ::-webkit-scrollbar-thumb:hover { background: var(--text-tertiary); }
        
        /* Tab导航样式 */
        .tab-nav {
            display: flex;
            gap: 8px;
            padding: 16px 0;
            overflow-x: auto;
            flex-wrap: wrap;
        }
        .tab-btn {
            padding: 10px 20px;
            border: none;
            border-radius: 10px;
            background: var(--bg-tertiary);
            color: var(--text-secondary);
            font-size: 0.9rem;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.3s;
            white-space: nowrap;
            border: 1px solid var(--border-color);
        }
        .tab-btn:hover {
            background: var(--bg-secondary);
            border-color: var(--primary);
            color: var(--primary);
        }
        .tab-btn.active {
            background: linear-gradient(135deg, var(--gradient-start) 0%, var(--gradient-end) 100%);
            color: #fff;
            border-color: transparent;
            box-shadow: 0 4px 12px rgba(99,102,241,0.3);
        }
        .dark .tab-btn {
            background: var(--bg-tertiary);
            border-color: var(--border-color);
        }
        .dark .tab-btn.active {
            background: linear-gradient(135deg, rgba(90,123,255,0.3) 0%, rgba(139,92,246,0.3) 100%);
            border-color: rgba(139,92,246,0.5);
        }
        .group-content { display: none; }
        .group-content.active { display: block; }
        .group-desc {
            color: var(--text-secondary);
            margin-bottom: 20px;
            font-size: 0.95rem;
        }
        
        /* 关于页面样式 */
        .about-section { display: flex; flex-direction: column; gap: 24px; }
        .about-intro .about-description { line-height: 1.8; color: var(--text-secondary); }
        .about-intro .about-description p { margin-bottom: 12px; }
        .about-intro .about-description a { color: var(--primary); text-decoration: none; }
        .about-intro .about-description a:hover { text-decoration: underline; }
        
        .about-links .links-grid {
            display: flex;
            flex-wrap: wrap;
            gap: 12px;
            margin-top: 16px;
        }
        .link-item {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 12px 20px;
            background: var(--bg-tertiary);
            border-radius: 12px;
            color: var(--text-primary);
            text-decoration: none;
            transition: all 0.3s;
            border: 1px solid var(--border-color);
        }
        .link-item:hover {
            background: linear-gradient(135deg, rgba(90,123,255,0.1) 0%, rgba(139,92,246,0.1) 100%);
            border-color: var(--primary);
            transform: translateY(-2px);
        }
        .link-item i { color: var(--primary); }
        
        .about-donate .donate-desc {
            color: var(--text-secondary);
            margin-bottom: 20px;
        }
        .donate-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
            gap: 20px;
        }
        .donate-item {
            background: var(--bg-tertiary);
            border-radius: 16px;
            padding: 20px;
            text-align: center;
            border: 2px solid var(--border-color);
            transition: all 0.3s;
        }
        .donate-item:hover {
            transform: translateY(-4px);
            box-shadow: var(--card-shadow-hover);
        }
        .donate-header {
            display: flex;
            align-items: center;
            justify-content: center;
            gap: 8px;
            font-weight: 600;
            margin-bottom: 16px;
        }
        .donate-header i { font-size: 1.25rem; }
        .donate-qrcode {
            max-width: 160px;
            width: 100%;
            border-radius: 8px;
        }
        
        .ads-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
            gap: 16px;
            margin-top: 16px;
        }
        .ad-item {
            display: flex;
            align-items: center;
            gap: 16px;
            padding: 16px;
            background: var(--bg-tertiary);
            border-radius: 12px;
            text-decoration: none;
            color: var(--text-primary);
            border: 2px solid var(--border-color);
            transition: all 0.3s;
        }
        .ad-item:hover {
            transform: translateY(-2px);
            box-shadow: var(--card-shadow-hover);
        }
        .ad-image {
            width: 60px;
            height: 60px;
            border-radius: 8px;
            object-fit: cover;
        }
        .ad-content h4 { margin-bottom: 4px; font-weight: 600; }
        .ad-content p { font-size: 0.85rem; color: var(--text-secondary); margin: 0; }
        
        .about-custom-content {
            background: var(--bg-secondary);
            border-radius: 20px;
            padding: 28px;
            box-shadow: var(--card-shadow);
            border: 1px solid var(--border-color);
        }
        .about-custom-content img { max-width: 100%; height: auto; }
        
        .about-disclaimer {
            border-left: 4px solid var(--warning);
        }
        .disclaimer-content {
            color: var(--text-secondary);
            line-height: 1.8;
            font-size: 0.9rem;
        }
        .disclaimer-content p { margin-bottom: 8px; }
        .ai-generated-badge {
            display: inline-flex;
            align-items: center;
            gap: 8px;
            margin-top: 16px;
            padding: 10px 16px;
            background: linear-gradient(135deg, rgba(139,92,246,0.1) 0%, rgba(99,102,241,0.1) 100%);
            border-radius: 20px;
            font-size: 0.85rem;
            color: var(--primary);
            border: 1px solid rgba(139,92,246,0.2);
        }
        .ai-generated-badge i { font-size: 1rem; }
    </style>
</head>
<body>
    <div class="toast-container" id="toastContainer"></div>
    <header>
        <div class="container">
            <div class="header-content">
                <div class="logo">
                    {{if isImage .Site.LogoIcon}}
                        <img src="{{.Site.LogoIcon}}" alt="Logo" style="height: 32px; width: auto; vertical-align: middle; margin-right: 8px;">
                    {{else}}
                        <i class="{{.Site.LogoIcon}}"></i>
                    {{end}}
                    <span>{{.Site.Logo}}</span>
                </div>
                {{if .SiteNotice.Enabled}}
                <a href="{{.SiteNotice.URL}}" target="_blank" rel="noopener" class="top-notice" style="{{if .SiteNotice.Color}}color: {{.SiteNotice.Color}};{{end}}{{if .SiteNotice.BgColor}}background: {{.SiteNotice.BgColor}};{{end}}">
                    {{if .SiteNotice.Icon}}<i class="{{.SiteNotice.Icon}}"></i> {{end}}{{.SiteNotice.Text}}
                </a>
                {{end}}
                <div class="header-actions">
                    <div class="ws-status">
                        <span class="ws-dot" id="ws-dot"></span>
                        <span id="ws-text">连接中...</span>
                    </div>
                    <div class="theme-selector">
                        <button class="theme-selector-btn" id="theme-selector-btn" title="选择主题">
                            <i class="fa fa-paint-brush"></i>
                            <span id="current-theme-name">紫蓝渐变</span>
                            <i class="fa fa-angle-down"></i>
                        </button>
                        <div class="theme-dropdown" id="theme-dropdown">
                            <div class="theme-option active" data-theme="default">
                                <span class="theme-preview preview-default"></span>
                                <span>紫蓝渐变</span>
                            </div>
                            <div class="theme-option" data-theme="fresh-blue">
                                <span class="theme-preview preview-fresh-blue"></span>
                                <span>清新蓝</span>
                            </div>
                        </div>
                    </div>
                    <button id="theme-toggle" title="切换深色/浅色模式">
                        <i class="fa fa-moon"></i>
                    </button>
                </div>
            </div>
        </div>
    </header>

    <main class="container">
        <h1 class="page-title">{{.Site.Title}}</h1>
        <p class="page-desc">{{.Site.Description}}</p>

        <!-- Tab导航 -->
        <nav class="tab-nav">
            {{range $index, $group := .Groups}}
            <button class="tab-btn{{if eq $index 0}} active{{end}}" data-group="{{$group.ID}}">{{$group.Name}}</button>
            {{end}}
            {{if .About.Enabled}}
            <button class="tab-btn" data-group="about"><i class="fa fa-info-circle"></i> {{if .About.Title}}{{.About.Title}}{{else}}关于{{end}}</button>
            {{end}}
        </nav>

        {{range $index, $group := .Groups}}
        <div class="group-content{{if eq $index 0}} active{{end}}" id="group-{{$group.ID}}" data-group-id="{{$group.ID}}" data-official-url="{{$group.OfficialURL}}">
            <p class="group-desc">{{$group.Description}}</p>

            <div class="stats-grid">
                <div class="stat-card gradient-card">
                    <div class="stat-card-content">
                        <div>
                            <p class="stat-label">总监控站点</p>
                            <h3 class="stat-value stat-total">{{$group.Total}}</h3>
                        </div>
                        <div class="stat-icon total"><i class="fa fa-sitemap"></i></div>
                    </div>
                </div>
                <div class="stat-card gradient-success">
                    <div class="stat-card-content">
                        <div>
                            <p class="stat-label">健康</p>
                            <h3 class="stat-value stat-healthy">{{$group.Healthy}}</h3>
                        </div>
                        <div class="stat-icon online"><i class="fa fa-check-circle"></i></div>
                    </div>
                </div>
                <div class="stat-card gradient-warning">
                    <div class="stat-card-content">
                        <div>
                            <p class="stat-label">慢速</p>
                            <h3 class="stat-value stat-slow">{{$group.Slow}}</h3>
                        </div>
                        <div class="stat-icon slow"><i class="fa fa-clock"></i></div>
                    </div>
                </div>
                <div class="stat-card gradient-danger">
                    <div class="stat-card-content">
                        <div>
                            <p class="stat-label">异常</p>
                            <h3 class="stat-value stat-abnormal">{{$group.Abnormal}}</h3>
                        </div>
                        <div class="stat-icon offline"><i class="fa fa-times-circle"></i></div>
                    </div>
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">镜像配置生成器</h3>
                    <div class="config-type-select">
                        <label>配置类型：</label>
                        <select class="config-type">
                            <option value="docker" selected>Docker daemon.json</option>
                            <option value="podman">Podman registries.conf</option>
                            <option value="containerd">Containerd config.toml</option>
                            <option value="nerdctl">Nerdctl</option>
                        </select>
                    </div>
                </div>

                <div class="mirror-selection-header">
                    <h4 class="mirror-selection-title">选择在线镜像源</h4>
                    <div class="mirror-selection-actions">
                        <button class="btn-sm btn-success use-recommended"><i class="fa fa-star"></i> 使用推荐配置</button>
                        <button class="btn-sm btn-primary select-all-online">全选在线</button>
                        <button class="btn-sm btn-secondary clear-selection">清空选择</button>
                    </div>
                </div>

                <div class="mirror-grid">
                    {{range $group.Results}}
                    <div class="mirror-option" data-name="{{.Name}}" data-url="{{.URL}}" data-status="{{.Status}}">
                        <input type="checkbox" class="mirror-checkbox">
                        <div class="mirror-info">
                            <div class="mirror-name-row">
                                <span class="mirror-status {{.Status}}"></span>
                                <span class="mirror-name">{{.Name}}</span>
                            </div>
                            <div class="mirror-url">{{.URL}}</div>
                        </div>
                    </div>
                    {{end}}
                </div>
                <p class="mirror-hint"><i class="fa fa-info-circle"></i> 建议选择 3-5 个镜像源以保证稳定性</p>

                <div class="config-output-header">
                    <h4 class="mirror-selection-title">生成的配置</h4>
                    <button class="btn-sm btn-success copy-config"><i class="fa fa-copy"></i> 复制配置</button>
                </div>
                <div class="code-block">
                    <pre><code class="config-output"></code></pre>
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">站点监控列表</h3>
                    <span class="last-check" style="font-size: 0.875rem; color: var(--text-secondary);">上次检查: {{$.LastCheck}}</span>
                </div>
                <div class="table-container">
                    <table>
                        <thead>
                            <tr>
                                <th style="width: 80px;">当前状态</th>
                                <th style="width: 160px;">站点名称</th>
                                <th style="width: 240px;">URL</th>
                                <th>响应时间</th>
                                <th style="width: 200px;">标签</th>
                                <th style="width: 160px;">最后监测时间</th>
                            </tr>
                        </thead>
                        <tbody class="status-tbody">
                            {{range $group.Results}}
                            <tr data-name="{{.Name}}">
                                <td><span class="status-dot {{.Status}}"></span></td>
                                <td class="site-name">{{.Name}}</td>
                                <td><span class="site-url" onclick="copyUrl('{{.URL}}')">{{.URL}}</span></td>
                                <td class="latency">{{if eq .LatencyMs -1}}--{{else}}{{.LatencyMs}} ms{{end}}</td>
                                <td>
                                    <div class="tags">
                                        {{range .Tags}}
                                        <span class="tag {{tagClass .}}">{{.}}</span>
                                        {{end}}
                                    </div>
                                </td>
                                <td class="check-time">{{formatTime .CheckedAt}}</td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>
                <div class="legend">
                    <span class="legend-item"><span class="status-dot healthy" style="width:10px;height:10px;"></span> 在线</span>
                    <span class="legend-item"><span class="status-dot slow" style="width:10px;height:10px;"></span> 响应缓慢</span>
                    <span class="legend-item"><span class="status-dot error" style="width:10px;height:10px;"></span> 离线</span>
                </div>
            </div>
        </div>
        {{end}}

        {{if .About.Enabled}}
        <!-- 关于页面 -->
        <div class="group-content" id="group-about" data-group-id="about">
            {{if .AboutContent}}
            <!-- 自定义HTML内容 -->
            <div class="about-custom-content">
                {{safeHTML .AboutContent}}
            </div>
            {{else}}
            <!-- 默认关于页面 -->
            <div class="about-section">
                <div class="about-intro card">
                    <h3 class="card-title"><i class="fa fa-info-circle"></i> 项目介绍</h3>
                    <div class="about-description">
                        {{if .About.Description}}{{safeHTML .About.Description}}{{else}}
                        <p>容器镜像监控系统是一个开源项目，帮助开发者实时监控各种容器镜像加速器的可用性和响应速度。</p>
                        {{end}}
                    </div>
                </div>

                {{if .About.Links}}
                <div class="about-links card">
                    <h3 class="card-title"><i class="fa fa-link"></i> 相关链接</h3>
                    <div class="links-grid">
                        {{range .About.Links}}
                        <a href="{{.URL}}" target="_blank" rel="noopener" class="link-item">
                            <i class="{{if .Icon}}{{.Icon}}{{else}}fa-external-link{{end}}"></i>
                            <span>{{.Name}}</span>
                        </a>
                        {{end}}
                    </div>
                </div>
                {{end}}

                {{if .About.Donate.Enabled}}
                <div class="about-donate card">
                    <h3 class="card-title"><i class="fa fa-heart"></i> {{if .About.Donate.Title}}{{.About.Donate.Title}}{{else}}支持本项目{{end}}</h3>
                    {{if .About.Donate.Description}}
                    <p class="donate-desc">{{.About.Donate.Description}}</p>
                    {{end}}
                    {{if .About.Donate.Items}}
                    <div class="donate-grid">
                        {{range .About.Donate.Items}}
                        <div class="donate-item" style="{{if .Color}}border-color: {{.Color}}{{end}}">
                            <div class="donate-header">
                                <i class="{{if .Icon}}{{.Icon}}{{else}}fa-gift{{end}}" style="{{if .Color}}color: {{.Color}}{{end}}"></i>
                                <span>{{.Name}}</span>
                            </div>
                            {{if .QRCode}}
                            <img src="{{.QRCode}}" alt="{{.Name}}" class="donate-qrcode">
                            {{end}}
                        </div>
                        {{end}}
                    </div>
                    {{end}}
                </div>
                {{end}}

                {{if .About.Ads.Enabled}}
                {{if .About.Ads.Items}}
                <div class="about-ads card">
                    <h3 class="card-title"><i class="fa fa-bullhorn"></i> 推荐服务</h3>
                    <div class="ads-grid">
                        {{range .About.Ads.Items}}
                        <a href="{{.URL}}" target="_blank" rel="noopener" class="ad-item" style="{{if .Color}}border-color: {{.Color}}{{end}}">
                            {{if .Image}}
                            <img src="{{.Image}}" alt="{{.Title}}" class="ad-image">
                            {{end}}
                            <div class="ad-content">
                                <h4>{{.Title}}</h4>
                                <p>{{.Description}}</p>
                            </div>
                        </a>
                        {{end}}
                    </div>
                </div>
                {{end}}
                {{end}}

                {{if .About.Disclaimer.Enabled}}
                <div class="about-disclaimer card">
                    <h3 class="card-title"><i class="fa fa-exclamation-triangle"></i> {{if .About.Disclaimer.Title}}{{.About.Disclaimer.Title}}{{else}}免责声明{{end}}</h3>
                    <div class="disclaimer-content">
                        {{if .About.Disclaimer.Content}}{{safeHTML .About.Disclaimer.Content}}{{end}}
                    </div>
                    {{if .About.Disclaimer.AIGenerated}}
                    <div class="ai-generated-badge">
                        <i class="fa fa-robot"></i> {{if .About.Disclaimer.AIStatement}}{{.About.Disclaimer.AIStatement}}{{else}}本项目由 AI 辅助生成{{end}}
                    </div>
                    {{end}}
                </div>
                {{end}}
            </div>
            {{end}}
        </div>
        {{end}}
    </main>

    <footer>
        <div class="container">
            <p>
                {{if .Footer.EnableHTML}}
                {{if .Footer.Text}}{{safeHTML .Footer.Text}}{{else}}容器镜像监控系统{{end}} | 实时更新中
                {{if .Footer.Copyright}}<br>{{safeHTML .Footer.Copyright}}{{end}}
                {{else}}
                {{if .Footer.Text}}{{.Footer.Text}}{{else}}容器镜像监控系统{{end}} | 实时更新中
                {{if .Footer.Copyright}}<br>{{.Footer.Copyright}}{{end}}
                {{end}}
            </p>
            {{if .Footer.ICP}}
            <p><a href="{{if .Footer.ICPURL}}{{.Footer.ICPURL}}{{else}}https://beian.miit.gov.cn/{{end}}" target="_blank" rel="noopener">{{.Footer.ICP}}</a></p>
            {{end}}
            {{if .Footer.Links}}
            <p>
                {{range $index, $link := .Footer.Links}}{{if $index}} | {{end}}<a href="{{$link.URL}}" target="_blank" rel="noopener">{{$link.Name}}</a>{{end}}
            </p>
            {{end}}
        </div>
    </footer>

    <script>
        // Toast 提示函数
        function showToast(message, type = 'success', duration = 2500) {
            const container = document.getElementById('toastContainer');
            const toast = document.createElement('div');
            toast.className = 'toast ' + type;
            toast.innerHTML = '<i class="fa ' + (type === 'success' ? 'fa-check-circle' : 'fa-times-circle') + '"></i><span>' + message + '</span>';
            container.appendChild(toast);
            
            setTimeout(() => {
                toast.classList.add('hiding');
                setTimeout(() => toast.remove(), 300);
            }, duration);
        }

        // 配置模板
        const configTemplates = {{json .ConfigTemplates}};

        // 推荐镜像配置
        const recommendedMirrors = {{json .RecommendedMirrors}};
        const recommendedOnlyOnline = {{.RecommendedOnlyOnline}};

        // Theme toggle (dark/light mode)
        const themeToggle = document.getElementById('theme-toggle');
        const themeIcon = themeToggle.querySelector('i');
        
        // Theme selector (color themes)
        const themeSelectorBtn = document.getElementById('theme-selector-btn');
        const themeDropdown = document.getElementById('theme-dropdown');
        const currentThemeName = document.getElementById('current-theme-name');
        const themeOptions = document.querySelectorAll('.theme-option');
        
        const themeNames = {
            'default': '紫蓝渐变',
            'fresh-blue': '清新蓝'
        };
        
        function setDarkMode(dark) {
            document.body.classList.toggle('dark', dark);
            themeIcon.className = dark ? 'fa fa-sun' : 'fa fa-moon';
            localStorage.setItem('darkMode', dark ? 'dark' : 'light');
        }
        
        function setColorTheme(theme) {
            // Remove all theme classes
            document.body.classList.remove('theme-fresh-blue');
            
            // Add new theme class if not default
            if (theme !== 'default') {
                document.body.classList.add('theme-' + theme);
            }
            
            // Update UI
            currentThemeName.textContent = themeNames[theme] || themeNames['default'];
            themeOptions.forEach(opt => {
                opt.classList.toggle('active', opt.dataset.theme === theme);
            });
            
            localStorage.setItem('colorTheme', theme);
        }
        
        // Initialize dark mode
        const savedDarkMode = localStorage.getItem('darkMode');
        if (savedDarkMode === 'dark' || (!savedDarkMode && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
            setDarkMode(true);
        }
        
        // Initialize color theme
        const savedColorTheme = localStorage.getItem('colorTheme');
        if (savedColorTheme && savedColorTheme !== 'default') {
            setColorTheme(savedColorTheme);
        }
        
        // Dark mode toggle click
        themeToggle.addEventListener('click', () => {
            setDarkMode(!document.body.classList.contains('dark'));
        });
        
        // Theme selector dropdown toggle
        themeSelectorBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            themeDropdown.classList.toggle('show');
        });
        
        // Theme option click
        themeOptions.forEach(opt => {
            opt.addEventListener('click', () => {
                setColorTheme(opt.dataset.theme);
                themeDropdown.classList.remove('show');
            });
        });
        
        // Close dropdown when clicking outside
        document.addEventListener('click', () => {
            themeDropdown.classList.remove('show');
        });

        // Tab切换
        let currentGroupId = document.querySelector('.tab-btn.active')?.dataset.group || '';
        
        document.querySelectorAll('.tab-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const groupId = btn.dataset.group;
                if (groupId === currentGroupId) return;
                
                // 更新Tab状态
                document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                
                // 切换内容
                document.querySelectorAll('.group-content').forEach(c => c.classList.remove('active'));
                document.getElementById('group-' + groupId).classList.add('active');
                
                currentGroupId = groupId;
                
                // 重新生成当前分组的配置（关于页面除外）
                if (groupId !== 'about') {
                    generateConfigForGroup(groupId);
                }
            });
        });

        // WebSocket
        const wsDot = document.getElementById('ws-dot');
        const wsText = document.getElementById('ws-text');
        let ws;
        let reconnectTimer;

        function connectWebSocket() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            ws = new WebSocket(protocol + '//' + window.location.host + '/ws');
            
            ws.onopen = () => {
                wsDot.classList.add('connected');
                wsText.textContent = '实时连接';
                console.log('WebSocket 已连接');
            };
            
            ws.onclose = () => {
                wsDot.classList.remove('connected');
                wsText.textContent = '重连中...';
                console.log('WebSocket 断开，5秒后重连');
                reconnectTimer = setTimeout(connectWebSocket, 5000);
            };
            
            ws.onerror = (err) => {
                console.error('WebSocket 错误:', err);
            };
            
            ws.onmessage = (event) => {
                const data = JSON.parse(event.data);
                updateUI(data);
            };
        }

        function updateUI(data) {
            // 更新所有分组的数据
            data.groups.forEach(group => {
                const groupEl = document.getElementById('group-' + group.id);
                if (!groupEl) return;
                
                // 更新统计
                groupEl.querySelector('.stat-total').textContent = group.total;
                groupEl.querySelector('.stat-healthy').textContent = group.healthy;
                groupEl.querySelector('.stat-slow').textContent = group.slow;
                groupEl.querySelector('.stat-abnormal').textContent = group.abnormal;
                groupEl.querySelector('.last-check').textContent = '上次检查: ' + data.checked_at;

                // 更新表格和镜像选择
                group.results.forEach(result => {
                    // 更新表格行
                    const row = groupEl.querySelector('tr[data-name="' + result.name + '"]');
                    if (row) {
                        const statusDot = row.querySelector('.status-dot');
                        const oldStatus = statusDot.className.split(' ').find(c => ['healthy', 'slow', 'timeout', 'error'].includes(c));
                        if (oldStatus !== result.status) {
                            statusDot.className = 'status-dot ' + result.status;
                            row.classList.add('update-flash');
                            setTimeout(() => row.classList.remove('update-flash'), 500);
                        }
                        
                        row.querySelector('.latency').textContent = result.latency_ms === -1 ? '--' : result.latency_ms + ' ms';
                        row.querySelector('.check-time').textContent = result.checked_at.replace('T', ' ').substring(0, 19);
                    }

                    // 更新镜像选择状态
                    const mirrorOpt = groupEl.querySelector('.mirror-option[data-name="' + result.name + '"]');
                    if (mirrorOpt) {
                        mirrorOpt.dataset.status = result.status;
                        const statusIndicator = mirrorOpt.querySelector('.mirror-status');
                        statusIndicator.className = 'mirror-status ' + result.status;
                    }
                });
            });
        }

        connectWebSocket();

        // 获取分组的registry域名
        function getRegistryDomain(groupId) {
            const groupEl = document.getElementById('group-' + groupId);
            if (!groupEl) return 'docker.io';
            const officialUrl = groupEl.dataset.officialUrl || '';
            try {
                const url = new URL(officialUrl);
                return url.host;
            } catch {
                return 'docker.io';
            }
        }

        // Config generator for specific group
        function getSelectedMirrorsForGroup(groupId) {
            const groupEl = document.getElementById('group-' + groupId);
            if (!groupEl) return [];
            const selected = [];
            groupEl.querySelectorAll('.mirror-option').forEach(opt => {
                if (opt.querySelector('.mirror-checkbox').checked) {
                    selected.push(opt.dataset.url);
                }
            });
            return selected;
        }

        function generateConfigForGroup(groupId) {
            const groupEl = document.getElementById('group-' + groupId);
            if (!groupEl) return;
            
            const mirrors = getSelectedMirrorsForGroup(groupId);
            const configType = groupEl.querySelector('.config-type');
            const configOutput = groupEl.querySelector('.config-output');
            const type = configType ? configType.value : 'docker';
            const registry = getRegistryDomain(groupId);
            let config = '';

            if (mirrors.length === 0) {
                configOutput.innerHTML = '<span class="comment">' + configTemplates.empty_hint + '</span>';
                return;
            }

            // 生成步骤注释的辅助函数
            function buildStepsComment(tpl, registry) {
                let header = '<span class="comment">################################################################################</span>\n';
                header += '<span class="comment"># ' + tpl.title + ' (' + registry + ')</span>\n';
                header += '<span class="comment">################################################################################</span>\n';
                header += '<span class="comment">#</span>\n';
                header += '<span class="comment"># 使用步骤:</span>\n';
                tpl.steps.forEach((step, i) => {
                    header += '<span class="comment">#   ' + (i + 1) + '. ' + step.replace('{registry}', registry) + '</span>\n';
                });
                header += '<span class="comment">################################################################################</span>\n\n';
                return header;
            }

            if (type === 'docker') {
                const tpl = configTemplates.docker;
                config = buildStepsComment(tpl, registry);
                config += '{\n';
                config += '    <span class="key">"registry-mirrors"</span>: [\n';
                mirrors.forEach((m, i) => {
                    config += '        <span class="string">"' + m + '"</span>' + (i < mirrors.length - 1 ? ',' : '') + '\n';
                });
                config += '    ]\n';
                config += '}';
            } else if (type === 'podman') {
                const tpl = configTemplates.podman;
                config = buildStepsComment(tpl, registry);
                config += '<span class="key">unqualified-search-registries</span> = [<span class="string">"' + registry + '"</span>]\n\n';
                config += '[[<span class="key">registry</span>]]\n';
                config += '<span class="key">prefix</span> = <span class="string">"' + registry + '"</span>\n';
                config += '<span class="key">location</span> = <span class="string">"' + registry + '"</span>\n\n';
                mirrors.forEach(m => {
                    config += '[[<span class="key">registry.mirror</span>]]\n';
                    config += '<span class="key">location</span> = <span class="string">"' + m.replace('https://', '') + '"</span>\n\n';
                });
            } else if (type === 'containerd') {
                const tpl = configTemplates.containerd;
                config = buildStepsComment(tpl, registry);
                config += '[<span class="key">plugins."io.containerd.grpc.v1.cri".registry.mirrors."' + registry + '"</span>]\n';
                config += '  <span class="key">endpoint</span> = [\n';
                mirrors.forEach((m, i) => {
                    config += '    <span class="string">"' + m + '"</span>' + (i < mirrors.length - 1 ? ',' : '') + '\n';
                });
                config += '  ]';
            } else if (type === 'nerdctl') {
                const tpl = configTemplates.nerdctl;
                config = buildStepsComment(tpl, registry);
                config += '<span class="key">server</span> = <span class="string">"https://' + registry + '"</span>\n\n';
                mirrors.forEach(m => {
                    config += '[<span class="key">host.<span class="string">"' + m + '"</span></span>]\n';
                    config += '  <span class="key">capabilities</span> = [<span class="string">"pull"</span>, <span class="string">"resolve"</span>]\n\n';
                });
            }

            configOutput.innerHTML = config;
        }

        // 为每个分组初始化事件监听
        document.querySelectorAll('.group-content').forEach(groupEl => {
            const groupId = groupEl.dataset.groupId;
            
            // 镜像选择点击
            groupEl.querySelectorAll('.mirror-option').forEach(opt => {
                opt.addEventListener('click', (e) => {
                    if (e.target.type !== 'checkbox') {
                        const cb = opt.querySelector('.mirror-checkbox');
                        cb.checked = !cb.checked;
                    }
                    opt.classList.toggle('selected', opt.querySelector('.mirror-checkbox').checked);
                    generateConfigForGroup(groupId);
                });
            });

            // 配置类型切换
            const configType = groupEl.querySelector('.config-type');
            if (configType) {
                configType.addEventListener('change', () => generateConfigForGroup(groupId));
            }

            // 使用推荐配置
            groupEl.querySelector('.use-recommended')?.addEventListener('click', () => {
                const recommended = recommendedMirrors && recommendedMirrors[groupId];
                
                if (recommended && recommended.length > 0) {
                    // 使用配置的推荐列表
                    groupEl.querySelectorAll('.mirror-option').forEach(opt => {
                        const cb = opt.querySelector('.mirror-checkbox');
                        const url = opt.dataset.url;
                        const isOnline = opt.dataset.status === 'healthy';
                        // 检查URL是否在推荐列表中（忽略末尾斜杠差异）
                        const isRecommended = recommended.some(r => 
                            url === r || url === r + '/' || url + '/' === r || 
                            url.replace(/\/$/, '') === r.replace(/\/$/, '')
                        );
                        // 根据配置决定是否只选择在线镜像
                        cb.checked = recommendedOnlyOnline ? (isOnline && isRecommended) : isRecommended;
                        opt.classList.toggle('selected', cb.checked);
                    });
                } else {
                    // 按配置顺序选择前5个在线的
                    let count = 0;
                    groupEl.querySelectorAll('.mirror-option').forEach(opt => {
                        const cb = opt.querySelector('.mirror-checkbox');
                        const isOnline = opt.dataset.status === 'healthy';
                        cb.checked = isOnline && count < 5;
                        if (cb.checked) count++;
                        opt.classList.toggle('selected', cb.checked);
                    });
                }
                generateConfigForGroup(groupId);
            });

            // 全选在线
            groupEl.querySelector('.select-all-online')?.addEventListener('click', () => {
                groupEl.querySelectorAll('.mirror-option').forEach(opt => {
                    const cb = opt.querySelector('.mirror-checkbox');
                    const isOnline = opt.dataset.status === 'healthy';
                    cb.checked = isOnline;
                    opt.classList.toggle('selected', cb.checked);
                });
                generateConfigForGroup(groupId);
            });

            // 清空选择
            groupEl.querySelector('.clear-selection')?.addEventListener('click', () => {
                groupEl.querySelectorAll('.mirror-checkbox').forEach(cb => {
                    cb.checked = false;
                    cb.closest('.mirror-option').classList.remove('selected');
                });
                generateConfigForGroup(groupId);
            });

            // 复制配置
            groupEl.querySelector('.copy-config')?.addEventListener('click', () => {
                const text = groupEl.querySelector('.config-output').innerText;
                navigator.clipboard.writeText(text).then(() => {
                    showToast('配置已复制到剪贴板');
                });
            });

            // 初始化配置显示
            generateConfigForGroup(groupId);
        });

        function copyUrl(url) {
            // 优先尝试现代 API
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(url).then(() => {
                    showToast('URL 已复制');
                }).catch(err => {
                    console.error('Clipboard API failed:', err);
                    fallbackCopyTextToClipboard(url);
                });
            } else {
                // 回退到旧版 API
                fallbackCopyTextToClipboard(url);
            }
        }

        function fallbackCopyTextToClipboard(text) {
            var textArea = document.createElement("textarea");
            textArea.value = text;
            
            // 避免滚动到底部
            textArea.style.top = "0";
            textArea.style.left = "0";
            textArea.style.position = "fixed";

            document.body.appendChild(textArea);
            textArea.focus();
            textArea.select();

            try {
                var successful = document.execCommand('copy');
                if (successful) {
                    showToast('URL 已复制');
                } else {
                    showToast('复制失败，请手动复制', 'error');
                }
            } catch (err) {
                console.error('Fallback: Oops, unable to copy', err);
                showToast('复制失败，请手动复制', 'error');
            }

            document.body.removeChild(textArea);
        }
    </script>
</body>
</html>`

type PageData struct {
	Groups                []GroupStatus
	TagColors             []TagColor
	Site                  Site
	SiteNotice            SiteNotice
	Footer                Footer
	ConfigTemplates       ConfigTemplates
	RecommendedMirrors    map[string][]string
	RecommendedOnlyOnline bool
	About                 About
	AboutContent          string // 自定义HTML文件的内容
	LastCheck             string
}

func tagClass(tag string) string {
	tagLower := strings.ToLower(tag)
	switch {
	case strings.Contains(tagLower, "cloudflare"):
		return "tag-cloudflare"
	case strings.Contains(tagLower, "nginx"):
		return "tag-nginx"
	case strings.Contains(tagLower, "阿里"):
		return "tag-aliyun"
	case strings.Contains(tagLower, "腾讯"):
		return "tag-tencent"
	case strings.Contains(tagLower, "1panel"):
		return "tag-1panel"
	case strings.Contains(tagLower, "daocloud"):
		return "tag-daocloud"
	case strings.Contains(tagLower, "需") || strings.Contains(tagLower, "限"):
		return "tag-warning"
	default:
		return "tag-default"
	}
}

func makeTagClassFunc(tagColors []TagColor) func(string) string {
	return func(tag string) string {
		tagLower := strings.ToLower(tag)
		// 先检查自定义颜色配置
		for _, tc := range tagColors {
			if strings.Contains(tagLower, strings.ToLower(tc.Keyword)) {
				return "tag-custom-" + tc.Keyword
			}
		}
		// 回退到默认规则
		return tagClass(tag)
	}
}

// 判断字符串是否为图片路径、URL 或 Base64
func isImage(s string) bool {
	s = strings.ToLower(s)
	// 只要符合以下特征之一，就认为是图片，渲染为 <img> 标签
	return strings.HasPrefix(s, "http") || // 网络图片 http://...
		strings.HasPrefix(s, "/") || // 本地路径 /custom/...
		strings.HasPrefix(s, "data:image") || // Base64 编码
		strings.HasSuffix(s, ".png") ||
		strings.HasSuffix(s, ".svg") ||
		strings.HasSuffix(s, ".jpg") ||
		strings.HasSuffix(s, ".jpeg") ||
		strings.HasSuffix(s, ".ico") ||
		strings.HasSuffix(s, ".webp") ||
		strings.HasSuffix(s, ".gif")
}

const (
	// 允许等待 Pong 消息的最大时间
	pongWait = 60 * time.Second
	// 发送 Ping 消息的周期 (必须小于 pongWait)
	pingPeriod = (pongWait * 9) / 10
	// 允许写入消息的最大时间
	writeWait = 10 * time.Second
	// 允许读取的最大消息尺寸 (字节)
	maxMessageSize = 512
)

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// 全局调试开关
var debugMode bool

// 仅在调试模式下打印日志
func logDebug(format string, v ...interface{}) {
	if debugMode {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// 禁止目录列表的文件系统
type neuteredFileSystem struct {
	fs http.FileSystem
}

func (nfs neuteredFileSystem) Open(path string) (http.File, error) {
	f, err := nfs.fs.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// 如果是目录，检查是否有 index.html，如果没有则禁止访问
	if s.IsDir() {
		index := strings.TrimSuffix(path, "/") + "/index.html"
		if _, err := nfs.fs.Open(index); err != nil {
			return nil, os.ErrPermission // 返回 403 Forbidden
		}
	}

	return f, nil
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 允许同源请求以及常见开发环境
		origin := r.Header.Get("Origin")
		host := r.Host

		// 检查是否与主机匹配（包括协议）
		if origin != "" {
			expectedOrigin := "http://" + host
			if r.TLS != nil {
				expectedOrigin = "https://" + host
			}

			return origin == expectedOrigin ||
				origin == strings.Replace(expectedOrigin, "http://", "ws://", 1) ||
				origin == strings.Replace(expectedOrigin, "https://", "wss://", 1)
		}

		// 如果没有 Origin 头，可能是直接连接，允许
		return true
	},
}

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	configShort := flag.String("c", "config.yaml", "配置文件路径")
	flag.Parse()

	// 使用 -c 参数优先，如果没有指定 -c 则使用 --config
	actualConfigPath := *configPath
	if *configShort != "config.yaml" || *configPath == "config.yaml" {
		actualConfigPath = *configShort
	}

	// 检查是否有额外的命令行参数
	args := flag.Args()
	if len(args) > 0 {
		if args[0] == "healthcheck" {
			// 设置默认值
			listenAddr := ":9080"
			useTLS := false

			// 确保能读取到 -config 参数，无论它在 healthcheck 命令的前面还是后面
			configToLoad := actualConfigPath
			for i, arg := range os.Args {
				// 情况1: -c value 或 -config value 或 --config value
				if (arg == "-config" || arg == "-c" || arg == "--config") && i+1 < len(os.Args) {
					configToLoad = os.Args[i+1]
					break
				}

				// 情况2: -c=value 或 -config=value 或 --config=value
				if strings.HasPrefix(arg, "-config=") || strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "-c=") {
					parts := strings.SplitN(arg, "=", 2)
					if len(parts) == 2 {
						configToLoad = parts[1]
						break
					}
				}
			}
			// 如果指定的文件不存在，尝试使用 Docker 默认路径兜底
			if _, err := os.Stat(configToLoad); os.IsNotExist(err) {
				if _, err := os.Stat("/app/data/config.yaml"); err == nil {
					configToLoad = "/app/data/config.yaml"
				}
			}

			// 尝试加载配置文件 (使用 configToLoad)
			if conf, err := LoadConfig(configToLoad); err == nil {
				if conf.Server.Listen != "" {
					listenAddr = conf.Server.Listen
				}
				useTLS = conf.Server.TLS.Enabled
			}

			// 规范化地址，确保客户端能连接
			if strings.HasPrefix(listenAddr, ":") {
				listenAddr = "127.0.0.1" + listenAddr
			} else if strings.HasPrefix(listenAddr, "0.0.0.0:") {
				listenAddr = strings.Replace(listenAddr, "0.0.0.0:", "127.0.0.1:", 1)
			}

			// 确定协议
			protocol := "http"
			if useTLS {
				protocol = "https"
			}

			// 拼接最终的健康检查 URL
			healthURL := fmt.Sprintf("%s://%s/healthz", protocol, listenAddr)

			// 创建 HTTP 客户端
			client := &http.Client{
				Timeout: 5 * time.Second,
			}

			// 如果启用了 HTTPS，忽略证书校验
			if useTLS {
				client.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				}
			}

			// 发起请求
			resp, err := client.Get(healthURL)
			if err != nil {
				fmt.Printf("Unhealthy: connection failed to %s: %v\n", healthURL, err)
				os.Exit(1)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Unhealthy: status code %d\n", resp.StatusCode)
				os.Exit(1)
			}

			os.Exit(0)
		}
	}

	config, err := LoadAndValidateConfig(actualConfigPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化全局调试开关
	debugMode = config.Server.Debug

	totalTargets := 0
	for _, g := range config.Groups {
		totalTargets += len(g.Targets)
	}
	log.Printf("加载了 %d 个分组, 共 %d 个监控目标", len(config.Groups), totalTargets)
	log.Printf("探测间隔: %v, 缓慢阈值: %dms", config.Server.RefreshInterval, config.Server.SlowThreshold)

	hub := NewHub()
	go hub.Run()

	monitor := NewMonitor(config, *configPath, hub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go monitor.Start(ctx)

	// WebSocket 心跳保活机制
	// 每 30 秒发送一次空更新或特定心跳包，防止连接因闲置被断开
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// 发送一个轻量级的心跳数据，或者复用状态数据
				// 这里为了简单，直接触发一次当前状态的广播（开销很小）
				monitor.broadcastStatus()
			case <-ctx.Done():
				return
			}
		}
	}()

	funcMap := template.FuncMap{
		"tagClass":   makeTagClassFunc(config.Server.TagColors),
		"formatTime": formatTime,
		"isImage":    isImage, // <--- [新增] 注册 isImage 函数
		"json": func(v interface{}) template.JS {
			b, err := json.Marshal(v)
			if err != nil {
				log.Printf("JSON序列化失败: %v", err)
				return template.JS("{}")
			}
			return template.JS(b)
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
	}

	tmpl, err := template.New("index").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		log.Fatalf("解析模板失败: %v", err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		groupStatuses := monitor.GetGroupStatuses()
		currentConfig := monitor.GetConfig()

		// 读取自定义HTML文件内容
		var aboutContent string
		if currentConfig.Server.About.Enabled && currentConfig.Server.About.CustomHTMLFile != "" {
			content, err := os.ReadFile(currentConfig.Server.About.CustomHTMLFile)
			if err != nil {
				log.Printf("读取自定义关于页面文件失败: %v", err)
			} else {
				aboutContent = string(content)
			}
		}

		data := PageData{
			Groups:                groupStatuses,
			TagColors:             currentConfig.Server.TagColors,
			Site:                  currentConfig.Server.Site,
			SiteNotice:            currentConfig.Server.SiteNotice,
			Footer:                currentConfig.Server.Footer,
			ConfigTemplates:       currentConfig.Server.ConfigTemplates,
			RecommendedMirrors:    currentConfig.Server.RecommendedMirrors,
			RecommendedOnlyOnline: currentConfig.Server.RecommendedOnlyOnline,
			About:                 currentConfig.Server.About,
			AboutContent:          aboutContent,
			LastCheck:             time.Now().Format("2006-01-02 15:04:05"),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("渲染模板失败: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logDebug("WebSocket升级失败: %v", err)
			return
		}

		// 1. 设置读取限制 (防大包攻击)
		conn.SetReadLimit(maxMessageSize)

		// 2. 设置读取超时 (防僵尸连接)
		conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error { 
			conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil 
		})

		hub.register <- conn

		// 发送初始状态
		groupStatuses := monitor.GetGroupStatuses()
		update := &StatusUpdate{
			Groups:    groupStatuses,
			CheckedAt: time.Now().Format("2006-01-02 15:04:05"),
		}
		
		// 写入也要加超时
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := conn.WriteJSON(update); err != nil {
			logDebug("WebSocket初始状态发送失败: %v", err)
			conn.Close()
			hub.unregister <- conn
			return
		}

		// 循环读取 (处理 Close 消息和 Pong)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logDebug("WebSocket连接异常关闭: %v", err)
				}
				hub.unregister <- conn
				conn.Close() // 确保关闭
				break
			}
		}
	})

	// 安全的静态资源服务
	// 只开放 ./data/public 目录，避免泄露 ./data 下的 config.yaml 和 ssl 证书
	// 假设程序运行目录是 /app，数据目录是 /app/data
	publicDir := "./data/public"

	// 1. 自动创建目录（如果不存在），防止报错
	if _, err := os.Stat(publicDir); os.IsNotExist(err) {
		_ = os.MkdirAll(publicDir, 0755)
	}

	// 2. 注册路由：将 URL 中的 /custom/ 映射到本地的 ./data/public/ 目录
	// 用户访问 http://site/custom/logo.png -> 读取容器内 /app/data/public/logo.png
	// 使用 neuteredFileSystem 包装 http.Dir 以禁止目录列表显示
	fs := neuteredFileSystem{http.Dir(publicDir)}
	http.Handle("/custom/", http.StripPrefix("/custom/", http.FileServer(fs)))

	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		groupStatuses := monitor.GetGroupStatuses()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"groups":     groupStatuses,
			"checked_at": time.Now(),
		})
	})

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// 配置热加载端点
	http.HandleFunc("/api/reload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// 验证API密钥
		token := r.Header.Get("Authorization")
		if token == "" {
			// 也尝试从查询参数获取
			token = r.URL.Query().Get("token")
		}

		if token != "" {
			// 移除 "Bearer " 前缀（如果存在）
			if strings.HasPrefix(token, "Bearer ") {
				token = strings.TrimPrefix(token, "Bearer ")
			}
		}

		// 如果配置了API密钥，必须匹配
		if monitor.config.Server.APIToken != "" && token != monitor.config.Server.APIToken {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "未授权访问",
			})
			return
		}

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "只支持 POST 请求",
			})
			return
		}

		if err := monitor.ReloadConfig(); err != nil {
			log.Printf("配置热加载失败: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}

		// 立即执行一次探测
		go monitor.ProbeAll()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "配置重载成功，正在重新探测",
		})
	})

	// 定义生产级 Server 配置
	srv := &http.Server{
		Addr:              config.Server.Listen,
		Handler:           nil, // 使用默认路由
		ReadHeaderTimeout: 5 * time.Second,  // 防止 Slowloris 攻击
		ReadTimeout:       15 * time.Second, // 防止读取主体过慢
		WriteTimeout:      15 * time.Second, // 防止响应写入阻塞
		IdleTimeout:       60 * time.Second, // Keep-Alive 超时
	}

	if config.Server.TLS.Enabled && config.Server.TLS.CertFile != "" && config.Server.TLS.KeyFile != "" {
		log.Printf("启用 HTTPS 模式")
		log.Printf("证书: %s", config.Server.TLS.CertFile)
		log.Printf("私钥: %s", config.Server.TLS.KeyFile)

		// 验证文件是否存在
		if _, err := os.Stat(config.Server.TLS.CertFile); err != nil {
			log.Fatalf("证书文件不存在: %v", err)
		}
		if _, err := os.Stat(config.Server.TLS.KeyFile); err != nil {
			log.Fatalf("私钥文件不存在: %v", err)
		}

		log.Printf("HTTPS 服务启动在 %s", config.Server.Listen)
		if err := srv.ListenAndServeTLS(config.Server.TLS.CertFile, config.Server.TLS.KeyFile); err != nil {
			log.Fatalf("HTTPS 服务器启动失败: %v", err)
		}
	} else {
		log.Printf("HTTP 服务启动在 %s", config.Server.Listen)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("HTTP 服务器启动失败: %v", err)
		}
	}
}
