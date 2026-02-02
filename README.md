# Docker Mirror Monitor

实时监控多种容器镜像源的加速器状态，支持 Docker Hub、GHCR、Quay、MCR、K8s、GCR、Elastic、NVCR 等主流镜像仓库。

> **声明**：本项目由 AI 辅助生成，仅供学习和研究使用。

## 目录

- [功能特性](#功能特性)
- [快速开始](#快速开始)
  - [方式一：Docker 运行（推荐）](#方式一docker-运行推荐)
  - [方式二：直接运行](#方式二直接运行)
  - [方式三：从源码编译](#方式三从源码编译)
- [YAML 配置详解](#yaml-配置详解)
  - [完整配置示例](#完整配置示例)
  - [server 配置项详解](#server-配置项详解)
  - [groups 监控分组配置](#groups-监控分组配置)
  - [配置校验规则](#配置校验规则)
- [自定义 Logo 图标](#自定义-logo-图标)
- [配置热加载](#配置热加载)
- [Docker 镜像编译](#docker-镜像编译)
  - [环境要求](#环境要求)
  - [单架构镜像编译](#单架构镜像编译)
  - [多架构镜像编译](#多架构镜像编译)
  - [推送到不同仓库](#推送到不同仓库)
  - [编译可执行文件](#编译可执行文件)
  - [支持的架构](#支持的架构)
- [GitHub Actions CI/CD](#github-actions-cicd)
- [Nginx 反向代理配置](#nginx-反向代理配置)
- [API 接口](#api-接口)
- [目录结构](#目录结构)
- [License](#license)
- [免责声明](#免责声明)

## 功能特性

- **多分组监控**：支持 8 种容器镜像源分组，Tab 切换查看
- **实时状态推送**：WebSocket 实时推送监控状态（支持心跳保活）
- **智能健康检查**：内置自适应健康检查命令，自动识别端口和协议
- **灵活 Logo 配置**：支持 Font Awesome 图标、网络图片、本地图片及 Base64
- **配置生成器**：一键生成 Docker/Podman/Containerd/Nerdctl 配置
- **自定义标签颜色**：通过 YAML 配置自定义 tag 显示颜色
- **响应时间监控**：显示延迟，超过阈值标记为缓慢（支持自定义阈值）
- **流量去敏化**：探测请求随机抖动，避免流量特征被识别
- **深色模式**：支持浅色/深色主题切换
- **代理支持**：支持 HTTP/HTTPS/SOCKS5 代理
- **自定义站点**：支持自定义 Logo、标题、描述、Favicon
- **顶部公告栏**：支持在 Header 显示推广链接或通知
- **页脚自定义**：支持 ICP 备案、版权信息、自定义链接、HTML 渲染
- **关于页面**：支持项目介绍、捐赠、广告、免责声明、自定义 HTML
- **配置热加载**：无需重启即可更新配置
- **安全加固**：防 Slowloris 攻击、目录列表泄露保护

## 快速开始

### 方式一：Docker 运行（推荐）

```bash
# 拉取镜像
docker pull moelin/docker-mirror-monitor:latest

# 运行（使用默认配置）
docker run -d -p 9080:9080 docker-mirror-monitor:latest

# 运行（使用自定义配置）
docker run -d \
  -p 9080:9080 \
  -v $(pwd)/config.yaml:/app/data/config.yaml \
  -v $(pwd)/data:/app/data \
  docker-mirror-monitor:latest

# 使用 docker-compose
cat > docker-compose.yaml << 'EOF'
version: '3'
services:
  monitor:
    image: docker-mirror-monitor:latest
    ports:
      - "9080:9080"
    volumes:
      # 挂载配置文件
      - ./config.yaml:/app/data/config.yaml
      # [新增] 挂载数据目录 (用于存放自定义 Logo 图片到 public/ 子目录)
      - ./data:/app/data
      # [可选] 如果启用 HTTPS，还需要挂载证书
      # - ./data/ca.crt:/app/data/ca.crt
      # - ./data/ca.key:/app/data/ca.key
    healthcheck:
      # 使用内置的智能健康检查命令
      # 它会自动读取配置文件中的端口和 TLS 设置
      test: ["CMD", "/app/docker-mirror-monitor", "healthcheck", "-config", "/app/data/config.yaml"]
      interval: 30s
      timeout: 10s   # 建议设置稍长，因为包含随机抖动
      retries: 3
      start_period: 5s
    restart: unless-stopped
EOF
docker-compose up -d
```

### 方式二：直接运行

```bash
# Linux (amd64 / x86_64)
./docker-mirror-monitor-linux-amd64 -config config.yaml

# Linux (arm64 / aarch64)
./docker-mirror-monitor-linux-arm64 -config config.yaml

# Linux (armv7 / 树莓派 3B 等)
./docker-mirror-monitor-linux-armv7 -config config.yaml

# Linux (ppc64le / IBM Power)
./docker-mirror-monitor-linux-ppc64le -config config.yaml

# Linux (s390x / IBM Z)
./docker-mirror-monitor-linux-s390x -config config.yaml

# Windows (amd64)
docker-mirror-monitor-windows-amd64.exe -config config.yaml

# macOS (Intel)
./docker-mirror-monitor-darwin-amd64 -config config.yaml

# macOS (Apple Silicon / M1/M2/M3)
./docker-mirror-monitor-darwin-arm64 -config config.yaml
```

### 方式三：从源码编译

```bash
# 克隆项目
git clone https://github.com/okxlin/docker-mirror-monitor
cd docker-mirror-monitor

# 编译当前平台
make build

# 编译所有平台
make build-all

# 运行
./docker-mirror-monitor -config config.yaml
```

---

## YAML 配置详解

配置文件 `config.yaml` 分为两大部分：`server`（服务器配置）和 `groups`（监控分组配置）。

### 完整配置示例

```yaml
server:
  listen: ":9080"              # 监听地址和端口
  debug: false                 # [新增] 调试模式：false=简洁日志，true=详细连接日志
  refresh_interval: 30s        # 全局刷新间隔
  slow_threshold: 3000         # 响应时间超过3000ms视为缓慢
  
  # 站点配置
  site:
    logo: "容器镜像监控"        # Header左侧Logo文字
    logo_icon: "fa-docker"      # Logo图标（Font Awesome）
    title: "容器镜像加速器监控"  # 页面主标题
    description: "实时监控多种容器镜像源的加速器状态"  # 页面描述
    browser_title: "容器镜像监控"  # 浏览器标签页标题
    favicon: ""                 # favicon图标URL
  
  # 顶部广告栏配置
  header_ad:
    enabled: false              # 是否启用
    text: "限时优惠！"          # 广告文字
    url: ""                     # 点击跳转链接
    icon: "fa-tag"              # Font Awesome图标
    color: "#ff6b6b"            # 文字颜色
    bg_color: "rgba(255,107,107,0.1)"  # 背景颜色
  
  # 代理配置 (支持 HTTP/HTTPS/SOCKS5)
  proxy: ""                    # 留空则不使用代理
  
  # 页脚配置
  footer:
    text: "容器镜像监控系统"
    icp: ""
    icp_url: "https://beian.miit.gov.cn/"
    copyright: ""
    enable_html: false         # 是否启用HTML渲染
    links:
      - name: "GitHub"
        url: "https://github.com/xxx"
  
  # 标签颜色配置 (按顺序优先匹配，keyword不区分大小写)
  tag_colors:
    # 官方源
    - keyword: "官方"
      color: "#4f6ef7"
      bg_color: "rgba(79,110,247,0.15)"
    
    # CDN/服务商
    - keyword: "CloudFlare"
      color: "#f6821e"
      bg_color: "rgba(246,130,30,0.15)"
    - keyword: "Nginx"
      color: "#44a047"
      bg_color: "rgba(68,160,71,0.15)"
    - keyword: "EdgeOne"
      color: "#006eff"
      bg_color: "rgba(0,110,255,0.15)"
    
    # 云厂商
    - keyword: "阿里"
      color: "#ff9900"
      bg_color: "rgba(255,153,0,0.15)"
    - keyword: "腾讯"
      color: "#00a4ff"
      bg_color: "rgba(0,164,255,0.15)"
    - keyword: "Azure"
      color: "#0078d4"
      bg_color: "rgba(0,120,212,0.15)"
    - keyword: "Google"
      color: "#4285f4"
      bg_color: "rgba(66,133,244,0.15)"
    
    # 警告类标签
    - keyword: "需"
      color: "#ff5722"
      bg_color: "rgba(255,87,34,0.15)"
    - keyword: "限"
      color: "#ff5722"
      bg_color: "rgba(255,87,34,0.15)"

groups:
  # Docker Hub 镜像源
  - id: "dockerhub"                          # 分组唯一ID (必填)
    name: "Docker Hub"                        # 分组显示名称 (必填)
    description: "Docker官方镜像仓库加速器"    # 分组描述 (可选)
    official_url: "https://registry-1.docker.io"  # 官方源地址 (可选)
    targets:                                  # 监控目标列表 (必填，至少1个)
      - name: "Docker Hub"                    # 目标名称 (必填)
        url: "https://registry-1.docker.io/v2/"  # 检测URL (必填)
        method: "HEAD"                        # HTTP方法 (可选，默认HEAD)
        timeout: 5s                           # 超时时间 (可选，默认5s)
        tags: ["官方", "CloudFront"]          # 标签列表 (可选)

      - name: "阿里云镜像"
        url: "https://registry.cn-hangzhou.aliyuncs.com/v2/"
        tags: ["阿里云", "需配置"]
  
  # GHCR 镜像源
  - id: "ghcr"
    name: "GHCR"
    description: "GitHub Container Registry 镜像加速器"
    official_url: "https://ghcr.io"
    targets:
      - name: "GHCR 官方"
        url: "https://ghcr.io/v2/"
        tags: ["官方", "Azure"]
```

### server 配置项详解

| 配置项 | 类型 | 说明 | 默认值 | 示例 |
|--------|------|------|--------|------|
| `listen` | string | 监听地址和端口 | `:9080` | `":8080"`, `"127.0.0.1:9080"` |
| `debug` | bool | 调试模式：false=简洁日志，true=详细连接日志 | `false` | `true`, `false` |
| `refresh_interval` | duration | 探测刷新间隔 | `30s` | `"10s"`, `"1m"`, `"30s"` |
| `slow_threshold` | int | 响应缓慢阈值(毫秒) | `3000` | `2000`, `5000` |
| `proxy` | string | 代理服务器地址 | `""` | `"http://127.0.0.1:7890"` |

#### proxy 代理配置

支持三种代理协议：

```yaml
# HTTP/HTTPS 代理
proxy: "http://127.0.0.1:7890"

# SOCKS5 代理（无认证）
proxy: "socks5://127.0.0.1:1080"

# SOCKS5 代理（带认证）
proxy: "socks5://username:password@127.0.0.1:1080"
```

#### site 站点配置

| 配置项 | 类型 | 说明 | 默认值 |
|--------|------|------|--------|
| `logo` | string | Header左侧Logo文字 | `"容器镜像监控"` |
| `logo_icon` | string | Logo图标（Font Awesome） | `"fa-docker"` |
| `title` | string | 页面主标题 | `"容器镜像加速器监控"` |
| `description` | string | 页面描述 | `"实时监控多种容器镜像源..."` |
| `browser_title` | string | 浏览器标签页标题 | `"容器镜像监控"` |
| `favicon` | string | Favicon图标URL | `""` |

```yaml
site:
  logo: "我的监控"
  logo_icon: "fa-server"
  title: "Docker镜像源监控"
  description: "自建镜像监控系统"
  browser_title: "镜像监控"
  favicon: "https://example.com/favicon.ico"
```

#### header_ad 顶部广告栏配置

| 配置项 | 类型 | 说明 | 默认值 |
|--------|------|------|--------|
| `enabled` | bool | 是否启用 | `false` |
| `text` | string | 广告文字 | `"限时优惠！"` |
| `url` | string | 点击跳转链接 | `""` |
| `icon` | string | Font Awesome图标 | `"fa-tag"` |
| `color` | string | 文字颜色 | `"#ff6b6b"` |
| `bg_color` | string | 背景颜色 | `"rgba(255,107,107,0.1)"` |

```yaml
header_ad:
  enabled: true
  text: "🎉 限时优惠活动"
  url: "https://example.com/promo"
  icon: "fa-gift"
  color: "#ff6b6b"
  bg_color: "rgba(255,107,107,0.12)"
```

#### footer 页脚配置

| 配置项 | 类型 | 说明 | 示例 |
|--------|------|------|------|
| `text` | string | 页脚标题 | `"容器镜像监控系统"` |
| `icp` | string | ICP备案号 | `"京ICP备12345678号"` |
| `icp_url` | string | ICP备案链接 | `"https://beian.miit.gov.cn/"` |
| `copyright` | string | 版权信息 | `"© 2024 Your Company"` |
| `enable_html` | bool | 是否启用HTML渲染 | `false` |
| `links` | list | 自定义链接 | 见下方示例 |

```yaml
footer:
  text: "容器镜像监控系统"
  icp: "京ICP备12345678号"
  icp_url: "https://beian.miit.gov.cn/"
  copyright: "© 2024 My Company"
  enable_html: true  # 启用后text和copyright支持HTML
  links:
    - name: "GitHub"
      url: "https://github.com/example/repo"
```

#### tag_colors 标签颜色配置

标签颜色按配置顺序优先匹配，keyword 不区分大小写。

| 配置项 | 类型 | 说明 | 是否必填 |
|--------|------|------|----------|
| `keyword` | string | 匹配关键词 | 必填 |
| `color` | string | 文字颜色(HEX) | 必填 |
| `bg_color` | string | 背景颜色(RGBA) | 可选 |

```yaml
tag_colors:
  # 精确匹配优先放前面
  - keyword: "CloudFlare"
    color: "#f6821e"
    bg_color: "rgba(246,130,30,0.15)"
  
  # 模糊匹配放后面
  - keyword: "阿里"           # 会匹配 "阿里云"、"阿里巴巴" 等
    color: "#ff9900"
    bg_color: "rgba(255,153,0,0.15)"
```

**内置颜色规则（可覆盖）：**
- 官方、CloudFlare、Nginx、EdgeOne
- 阿里、腾讯、Azure、Google、Oracle
- 1Panel、DaoCloud、Red Hat、NVIDIA、Elastic
- 需（需登录）、限（限速）等警告标签

#### config_templates 配置生成器模板

自定义配置生成器的标题和使用步骤说明，不配置则使用内置默认值。

#### about 关于页面配置

关于页面的详细配置选项，包括启用状态、标题、描述等。

### groups 监控分组配置

| 配置项 | 类型 | 说明 | 是否必填 |
|--------|------|------|----------|
| `id` | string | 分组唯一ID | 必填 |
| `name` | string | 分组显示名称 | 必填 |
| `description` | string | 分组描述 | 可选 |
| `official_url` | string | 官方源地址 | 可选 |
| `targets` | list | 监控目标列表 | 必填 |

#### targets 监控目标配置

| 配置项 | 类型 | 说明 | 默认值 |
|--------|------|------|--------|
| `name` | string | 目标名称 | 必填 |
| `url` | string | 检测URL | 必填 |
| `method` | string | HTTP方法 | `HEAD` |
| `timeout` | duration | 超时时间 | `5s` |
| `tags` | list | 标签列表 | `[]` |

```yaml
groups:
  - id: "dockerhub"
    name: "Docker Hub"
    description: "Docker官方镜像仓库加速器"
    official_url: "https://registry-1.docker.io"
    targets:
      - name: "Docker Hub 官方"
        url: "https://registry-1.docker.io/v2/"
        method: "HEAD"        # 可选: GET, HEAD, OPTIONS
        timeout: 5s           # 支持: 1s, 500ms, 1m 等格式
        tags: ["官方", "CloudFront"]
      
      - name: "DaoCloud 镜像"
        url: "https://docker.m.daocloud.io/v2/"
        tags: ["DaoCloud", "阿里云", "限速"]
```

### 配置校验规则

配置加载时会进行以下校验：

1. **分组校验**
   - `id` 不能为空且不能重复
   - `name` 不能为空
   - `targets` 至少有一个目标

2. **目标校验**
   - `name` 不能为空且在同一分组内不能重复
   - `url` 不能为空且格式必须有效

3. **服务器配置校验**
   - `refresh_interval` 不能小于 1s
   - `tag_colors` 的 `keyword` 和 `color` 不能为空

---

## Docker 镜像编译

### 环境要求

- Go 1.22+
- Docker 20.10+
- Docker Buildx（多架构编译）

### 单架构镜像编译

```bash
# 使用 Makefile（推荐）
make docker

# 或直接使用 docker build
docker build -t docker-mirror-monitor:latest .

# 查看生成的镜像
docker images | grep docker-mirror-monitor
```

**Dockerfile 说明：**

```dockerfile
# 构建阶段 - 使用 golang:1.24.12-alpine 作为编译环境
FROM --platform=$BUILDPLATFORM golang:1.24.12-alpine AS builder

# 运行阶段 - 使用精简的 alpine 镜像
FROM alpine:3.18
# 安装 ca-certificates（HTTPS支持）和 tzdata（时区支持）
# 默认时区设置为 Asia/Shanghai
```

### 多架构镜像编译

支持以下架构：
- `linux/amd64` - x86_64 服务器
- `linux/arm64` - ARM64 服务器/树莓派4
- `linux/arm/v7` - ARMv7 设备
- `linux/ppc64le` - PowerPC 64 LE
- `linux/s390x` - IBM Z 系列

#### 步骤一：配置 Docker Buildx

```bash
# 创建并使用新的 buildx 构建器
docker buildx create --use --name multi-builder

# 检查构建器状态
docker buildx inspect --bootstrap

# 查看支持的平台
docker buildx ls
```

#### 步骤二：构建多架构镜像

```bash
# 方式一：使用 Makefile（构建并推送到仓库）
make docker-multi

# 方式二：手动构建并推送
docker buildx build \
  --platform linux/amd64,linux/arm64,linux/arm/v7,linux/ppc64le,linux/s390x \
  -t your-registry/container-mirror-monitor:latest \
  -t your-registry/container-mirror-monitor:1.0.0 \
  --push .

# 方式三：仅构建 amd64 并加载到本地
docker buildx build \
  --platform linux/amd64 \
  -t container-mirror-monitor:latest \
  --load .
```

#### 步骤三：验证多架构镜像

```bash
# 查看镜像支持的架构
docker buildx imagetools inspect your-registry/container-mirror-monitor:latest

# 输出示例：
# Name: your-registry/container-mirror-monitor:latest
# MediaType: application/vnd.docker.distribution.manifest.list.v2+json
# Manifests:
#   - Platform: linux/amd64
#   - Platform: linux/arm64
#   - Platform: linux/arm/v7
```

### 推送到不同仓库

```bash
# 推送到 Docker Hub
docker buildx build --platform linux/amd64,linux/arm64 \
  -t username/container-mirror-monitor:latest \
  --push .

# 推送到阿里云容器镜像服务
docker buildx build --platform linux/amd64,linux/arm64 \
  -t registry.cn-hangzhou.aliyuncs.com/namespace/container-mirror-monitor:latest \
  --push .

# 推送到 GitHub Container Registry
docker buildx build --platform linux/amd64,linux/arm64 \
  -t ghcr.io/username/container-mirror-monitor:latest \
  --push .
```

### 编译可执行文件

```bash
# 编译当前平台
make build

# 编译所有平台（输出到 dist/ 目录）
make build-all

# 手动编译指定平台
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o docker-mirror-monitor main.go
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o docker-mirror-monitor main.go
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o docker-mirror-monitor.exe main.go
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o docker-mirror-monitor main.go
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o docker-mirror-monitor main.go
```

### 支持的架构

| 架构 | Docker Platform | 可执行文件 | 适用场景 |
|------|-----------------|------------|----------|
| x86_64 | linux/amd64 | docker-mirror-monitor-linux-amd64 | 大多数服务器/VPS |
| ARM64 | linux/arm64 | docker-mirror-monitor-linux-arm64 | ARM服务器/树莓派4/M1 Mac |
| ARMv7 | linux/arm/v7 | docker-mirror-monitor-linux-armv7 | 树莓派3/旧ARM设备 |
| PowerPC 64 LE | linux/ppc64le | docker-mirror-monitor-linux-ppc64le | IBM Power 服务器 |
| IBM Z | linux/s390x | docker-mirror-monitor-linux-s390x | IBM Z 大型机 |
| Windows x64 | - | docker-mirror-monitor-windows-amd64.exe | Windows 服务器 |
| macOS Intel | - | docker-mirror-monitor-darwin-amd64 | Intel Mac |
| macOS Apple Silicon | - | docker-mirror-monitor-darwin-arm64 | M1/M2/M3 Mac |

---

## GitHub Actions CI/CD

项目提供两个 GitHub Actions workflow，实现自动化构建和发布。

### Workflow 文件

```
.github/workflows/
├── build-docker-image.yml  # 发布版本时构建并推送 Docker 镜像
└── release-binaries.yml    # 手动触发或发布版本时编译二进制文件
```

### build-docker-image.yml - 构建 Docker 镜像

**触发条件：**
- 推送以 `v` 开头的 Tag（如 `v1.0.0`）
- 手动触发（workflow_dispatch）

**执行内容：**
1. **构建 Docker 镜像** - 支持多架构（linux/amd64, linux/arm64, linux/arm/v7, linux/ppc64le, linux/s390x）
2. **推送镜像** - 推送到 GitHub Container Registry (GHCR)

### release-binaries.yml - 编译二进制文件

**触发条件：**
- 推送以 `v` 开头的 Tag（如 `v1.0.0`）
- 手动触发（workflow_dispatch）

**执行内容：**
1. **编译二进制文件** - 8 个平台，生成 SHA256 校验文件
2. **创建 GitHub Release** - 自动生成 Release Notes，上传所有二进制文件

### 发布新版本

```bash
# 1. 确保代码已提交并推送
git add .
git commit -m "release: v1.0.0"
git push origin main

# 2. 创建并推送版本标签
git tag v1.0.0
git push origin v1.0.0

# 3. GitHub Actions 自动执行：
#    - 编译 8 个平台的二进制文件
#    - 创建 GitHub Release 并上传文件
#    - 构建并推送 Docker 多架构镜像到 GHCR
```

### 生成的产物

**二进制文件（GitHub Release）：**

| 文件名 | 平台 |
|--------|------|
| `docker-mirror-monitor-linux-amd64` | Linux x86_64 |
| `docker-mirror-monitor-linux-arm64` | Linux ARM64 |
| `docker-mirror-monitor-linux-armv7` | Linux ARMv7 |
| `docker-mirror-monitor-linux-ppc64le` | Linux PowerPC 64 LE |
| `docker-mirror-monitor-linux-s390x` | Linux IBM Z |
| `docker-mirror-monitor-windows-amd64.exe` | Windows x64 |
| `docker-mirror-monitor-darwin-amd64` | macOS Intel |
| `docker-mirror-monitor-darwin-arm64` | macOS Apple Silicon |

每个文件附带 `.sha256` 校验文件。

**Docker 镜像（GHCR）：**

```bash
# 拉取最新版本
docker pull ghcr.io/your-username/your-repo:latest

# 拉取指定版本
docker pull ghcr.io/your-username/your-repo:1.0.0

# 支持架构：linux/amd64, linux/arm64, linux/arm/v7, linux/ppc64le, linux/s390x
```

### 自定义 Docker 镜像仓库

如需推送到其他仓库（如 Docker Hub、阿里云），修改 `.github/workflows/release.yml`：

```yaml
# 添加额外的镜像仓库
- name: Docker meta
  id: meta
  uses: docker/metadata-action@v5
  with:
    images: |
      ghcr.io/${{ github.repository }}
      docker.io/your-username/container-mirror-monitor        # Docker Hub
      registry.cn-hangzhou.aliyuncs.com/namespace/app         # 阿里云

# 添加对应的登录步骤
- name: Login to Docker Hub
  uses: docker/login-action@v3
  with:
    username: ${{ secrets.DOCKERHUB_USERNAME }}
    password: ${{ secrets.DOCKERHUB_TOKEN }}

- name: Login to Aliyun
  uses: docker/login-action@v3
  with:
    registry: registry.cn-hangzhou.aliyuncs.com
    username: ${{ secrets.ALIYUN_USERNAME }}
    password: ${{ secrets.ALIYUN_PASSWORD }}
```

**需要配置的 Secrets：**

| Secret 名称 | 说明 |
|-------------|------|
| `GITHUB_TOKEN` | 自动提供，无需配置 |
| `DOCKERHUB_USERNAME` | Docker Hub 用户名（可选）|
| `DOCKERHUB_TOKEN` | Docker Hub Access Token（可选）|
| `ALIYUN_USERNAME` | 阿里云容器镜像服务用户名（可选）|
| `ALIYUN_PASSWORD` | 阿里云容器镜像服务密码（可选）|

### 版本标签规范

| 标签格式 | 说明 | 示例 |
|----------|------|------|
| `v*.*.*` | 正式版本 | `v1.0.0`, `v2.1.3` |
| `v*.*.*-rc*` | 候选版本 | `v1.0.0-rc1` |
| `v*.*.*-beta*` | 测试版本 | `v1.0.0-beta1` |
| `v*.*.*-alpha*` | 预览版本 | `v1.0.0-alpha1` |

带 `-rc`、`-beta`、`-alpha` 后缀的版本会自动标记为 Pre-release。

---

## Nginx 反向代理配置

如果需要通过 Nginx 反向代理访问服务，以下是推荐配置：

### 基础配置

```nginx
server {
    listen 80;
    server_name mirror.example.com;

    location / {
        proxy_pass http://127.0.0.1:9080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # WebSocket 支持（重要！实时状态推送需要）
    location /ws {
        proxy_pass http://127.0.0.1:9080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;  # WebSocket 长连接超时
    }
}
```

### HTTPS 配置（推荐）

```nginx
server {
    listen 80;
    server_name mirror.example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name mirror.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
    ssl_prefer_server_ciphers off;

    location / {
        proxy_pass http://127.0.0.1:9080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /ws {
        proxy_pass http://127.0.0.1:9080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;
    }
}
```

### 配置说明

| 配置项 | 说明 |
|--------|------|
| `proxy_http_version 1.1` | WebSocket 需要 HTTP/1.1 |
| `Upgrade` / `Connection` | WebSocket 握手必需的头部 |
| `proxy_read_timeout 86400` | 保持 WebSocket 长连接（24小时） |
| `X-Forwarded-Proto` | 传递原始协议（http/https） |

---

## API 接口

| 路径 | 方法 | 说明 |
|------|------|------|
| `/` | GET | Web 界面 |
| `/ws` | WebSocket | 实时状态推送 |
| `/api/status` | GET | JSON 状态接口 |
| `/api/reload` | POST | 热加载配置文件 |
| `/healthz` | GET | 健康检查 |

## 自定义 Logo 图标

在 `config.yaml` 的 `site.logo_icon` 字段中，支持以下 4 种格式：

1.  **Font Awesome 图标**（推荐）：
    ```yaml
    logo_icon: "fab fa-docker"  # 品牌图标
    logo_icon: "fas fa-server"  # 实心图标
    ```

2.  **网络图片**：
    ```yaml
    logo_icon: "https://example.com/my-logo.png"
    ```

3.  **本地图片**（需挂载）：

    将图片放入宿主机的 `data/public/` 目录下，并确保 Docker 挂载了 `/app/data` 目录。
    配置示例：
    ```yaml
    logo_icon: "/custom/logo.png"
    ```
    > **注意**：程序只开放容器内 `/app/data/public/` 目录的访问权限。

4.  **Base64 图片**：
    ```yaml
    logo_icon: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAA..."
    ```

## 配置热加载

修改 `config.yaml` 后，无需重启服务，调用 API 即可热加载：

```bash
# 热加载配置 (需要携带配置文件中定义的 api_token)
curl -X POST "http://localhost:9080/api/reload?token=test-token-for-api-auth"

# 或者使用 Header 方式：
# curl -H "Authorization: test-token-for-api-auth" -X POST http://localhost:9080/api/reload

# 成功响应
{"success":true,"message":"配置重载成功，正在重新探测"}

# 配置校验失败响应
{"success":false,"error":"配置校验失败: 分组 'xxx' 目标 'yyy': url 不能为空"}

```

## 目录结构

```
.
├── .github/
│   └── workflows/
│       ├── build-docker-image.yml # 构建 Docker 镜像并推送
│       └── release-binaries.yml   # 发布二进制文件到 Release
├── main.go               # 主程序（单文件，包含前端和后端）
├── config.yaml           # 配置文件
├── go.mod                # Go 依赖定义
├── go.sum                # Go 依赖校验
├── Dockerfile            # Docker 多阶段构建文件
├── Makefile              # 构建脚本
├── README.md             # 说明文档
└── data/                 # (可选) 数据目录，用于存放静态资源
    └── public/           # (可选) 存放自定义 Logo 图片的目录
```
## 参考致谢

本项目在开发过程中参考了以下优秀项目与站点，特此致谢：

- **[status.anye.xyz](https://status.anye.xyz/)**: 提供了优秀的界面设计参考
- **[mcwlgzs/docker-mirror-monitor](https://github.com/mcwlgzs/docker-mirror-monitor)**: Docker 镜像加速服务监控

## License

MIT License

## 免责声明

- 本项目由 AI 辅助生成，仅供学习和研究使用，不提供任何形式的担保。
- 镜像源的可用性和安全性由各镜像源提供方负责，使用前请自行评估风险。
- 本项目不对因使用镜像源而导致的任何损失承担责任。
