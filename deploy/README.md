# 部署指南

本文档介绍如何部署数牍隐私计算 AI Agent 系统。

## 部署架构

```
┌─────────────────────────────────────────┐
│         Docker Compose 环境              │
│                                         │
│  ┌────────────────┐  ┌───────────────┐ │
│  │  agent_proxy   │  │  tsql-agent   │ │
│  │  (Port 7000)   │→ │  (LangGraph)  │ │
│  └────────────────┘  └───────┬───────┘ │
│                              │         │
│                              ↓         │
│                    ┌──────────────────┐│
│                    │ Docker Socket    ││
│                    │ (动态创建容器)    ││
│                    └──────────────────┘│
└─────────────────────────────────────────┘
                     ↓
        ┌────────────────────────┐
        │  tsql_alice (动态)     │
        │  - MySQL               │
        │  - SCQL Broker/Engine  │
        │  - Privacy Engine      │
        └────────────────────────┘
```

## 快速部署

### 1. 环境准备

**系统要求**:
- Ubuntu 20.04+ / CentOS 7+
- Docker 20.10+
- Docker Compose 2.0+
- 至少 4GB 内存
- 至少 20GB 磁盘空间

**安装 Docker**:
```bash
# Ubuntu
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

# 启动 Docker
sudo systemctl start docker
sudo systemctl enable docker
```

**安装 Docker Compose**:
```bash
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose
```

### 2. 配置环境变量

```bash
cd deploy
cp .env.example .env
```

编辑 `.env` 文件：

```bash
# OpenAI 兼容 API 配置
OPENAI_API_KEY=sk-your-api-key
OPENAI_API_BASE=https://api.openai.com/v1
OPENAI_MODEL=gpt-4

# Agent Proxy 端口
AGENT_PROXY_PORT=7000
```

**推荐的 LLM 配置**:

<details>
<summary>DeepSeek (推荐，性价比高)</summary>

```bash
OPENAI_API_KEY=sk-your-deepseek-key
OPENAI_API_BASE=https://api.deepseek.com
OPENAI_MODEL=deepseek-chat
```
</details>

<details>
<summary>SiliconFlow (国内访问快)</summary>

```bash
OPENAI_API_KEY=sk-your-siliconflow-key
OPENAI_API_BASE=https://api.siliconflow.cn
OPENAI_MODEL=deepseek-ai/DeepSeek-V3
```
</details>

<details>
<summary>OpenAI GPT-4</summary>

```bash
OPENAI_API_KEY=sk-your-openai-key
OPENAI_API_BASE=https://api.openai.com/v1
OPENAI_MODEL=gpt-4
```
</details>

### 3. 配置服务地址

编辑 `config/config.json`：

```json
{
  "privacy_agent_url": "http://tsql:8123",
  "nexus_server_url": "http://your-nexus-server:8080"
}
```

**字段说明**:
- `privacy_agent_url`: LangGraph Agent 服务地址（容器内部地址）
- `nexus_server_url`: Nexus 文件系统服务器地址（需要替换为实际地址）

### 4. 构建镜像

```bash
# 构建 agent_proxy 镜像
cd ../agent_proxy
docker build -t agent_proxy:latest .

# 构建 langgraph agent 镜像
cd ../langgraph
docker build -t tsql-langgraph:latest .

# 构建 tsql 隐私计算镜像（如果需要）
cd ..
docker build -t tsql:latest -f Dockerfile .
```

### 5. 启动服务

```bash
cd deploy
docker-compose up -d
```

### 6. 验证部署

```bash
# 检查服务状态
docker-compose ps

# 查看日志
docker-compose logs -f

# 测试 agent_proxy
curl http://localhost:7000/register \
  -H "Content-Type: application/json" \
  -d '{"user_id":"test","nexus_key":"sk-test"}'
```

## 配置文件详解

### docker-compose.yml

```yaml
services:
  agent_proxy:
    image: agent_proxy:latest
    restart: unless-stopped
    ports:
      - "${AGENT_PROXY_PORT:-7000}:2024"
    volumes:
      - ./config:/app/config
    networks:
      - tsql-network

  tsql:
    image: tsql-langgraph:latest
    restart: unless-stopped
    environment:
      OPENAI_API_KEY: ${OPENAI_API_KEY:-}
      OPENAI_API_BASE: ${OPENAI_API_BASE:-}
      OPENAI_MODEL: ${OPENAI_MODEL:-}
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    networks:
      - tsql-network

networks:
  tsql-network:
    driver: bridge
```

**重要说明**:
- `agent_proxy` 暴露 7000 端口供外部访问
- `tsql` 需要挂载 Docker socket 以创建隐私计算容器
- 两个服务在同一网络中，可以互相访问

### .env

环境变量配置文件：

```bash
# LLM 配置
OPENAI_API_KEY=sk-xxx
OPENAI_API_BASE=https://api.openai.com/v1
OPENAI_MODEL=gpt-4

# 端口配置
AGENT_PROXY_PORT=7000
```

### config/config.json

服务配置文件：

```json
{
  "privacy_agent_url": "http://tsql:8123",
  "nexus_server_url": "http://nexus-server:8080"
}
```

### config/mappings.json

用户映射文件（自动生成）：

```json
{
  "user_to_agent_key": {
    "alice": "sk-alice-nexus-key",
    "bob": "sk-bob-nexus-key"
  }
}
```

## 服务管理

### 启动服务

```bash
docker-compose up -d
```

### 停止服务

```bash
docker-compose down
```

### 重启服务

```bash
docker-compose restart
```

### 查看日志

```bash
# 所有服务日志
docker-compose logs -f

# 特定服务日志
docker-compose logs -f agent_proxy
docker-compose logs -f tsql
```

### 更新服务

```bash
# 重新构建镜像
docker-compose build

# 重启服务
docker-compose up -d
```

## 生产环境部署

### 1. 使用外部数据库

建议使用外部 MySQL 而不是容器内 MySQL：

```yaml
services:
  tsql:
    environment:
      MYSQL_HOST: mysql.example.com
      MYSQL_PORT: 3306
      MYSQL_USER: scql
      MYSQL_PASSWORD: ${MYSQL_PASSWORD}
```

### 2. 配置持久化存储

```yaml
services:
  agent_proxy:
    volumes:
      - ./config:/app/config
      - ./logs:/app/logs  # 日志持久化

  tsql:
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./data:/app/data  # 数据持久化
```

### 3. 配置资源限制

```yaml
services:
  agent_proxy:
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 1G
        reservations:
          cpus: '0.5'
          memory: 512M

  tsql:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 4G
        reservations:
          cpus: '1'
          memory: 2G
```

### 4. 配置健康检查

```yaml
services:
  agent_proxy:
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:2024/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  tsql:
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8123/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

### 5. 使用 HTTPS

建议使用 Nginx 作为反向代理：

```nginx
server {
    listen 443 ssl;
    server_name privacy-agent.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:7000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## 监控和日志

### 日志位置

- **Agent Proxy**: `agent_proxy.log`
- **LangGraph Agent**: Docker logs
- **Privacy Computing**: 容器内 `tsqlctl.log`

### 日志收集

使用 Docker 日志驱动：

```yaml
services:
  agent_proxy:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
```

### 监控指标

建议添加 Prometheus 监控：

```yaml
services:
  prometheus:
    image: prom/prometheus
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"
```

## 故障排查

### 问题 1: 服务无法启动

```bash
# 检查端口占用
sudo netstat -tlnp | grep 7000

# 检查 Docker 状态
sudo systemctl status docker

# 查看详细日志
docker-compose logs
```

### 问题 2: 无法创建隐私计算容器

```bash
# 检查 Docker socket 权限
ls -l /var/run/docker.sock

# 检查 tsql 镜像
docker images | grep tsql

# 查看 tsql 服务日志
docker-compose logs tsql
```

### 问题 3: LLM 调用失败

```bash
# 测试 API 连接
curl -X POST ${OPENAI_API_BASE}/chat/completions \
  -H "Authorization: Bearer ${OPENAI_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"${OPENAI_MODEL}","messages":[{"role":"user","content":"test"}]}'
```

## 备份和恢复

### 备份配置

```bash
# 备份配置文件
tar -czf backup-$(date +%Y%m%d).tar.gz \
  deploy/.env \
  deploy/config/
```

### 恢复配置

```bash
# 解压备份
tar -xzf backup-20240101.tar.gz

# 重启服务
docker-compose restart
```

## 升级指南

### 升级步骤

1. **备份当前配置**
2. **拉取最新代码**
3. **重新构建镜像**
4. **更新服务**

```bash
# 备份
tar -czf backup-$(date +%Y%m%d).tar.gz deploy/

# 拉取代码
git pull

# 重新构建
cd agent_proxy && docker build -t agent_proxy:latest .
cd ../langgraph && docker build -t tsql-langgraph:latest .

# 更新服务
cd ../deploy
docker-compose down
docker-compose up -d
```

## 安全建议

1. **API Key 保护**: 不要将 API Key 提交到版本控制
2. **网络隔离**: 使用防火墙限制访问
3. **定期更新**: 及时更新 Docker 镜像和依赖
4. **日志审计**: 定期检查日志文件
5. **备份策略**: 定期备份配置和数据

## 性能调优

1. **增加资源**: 根据负载调整 CPU 和内存限制
2. **使用 SSD**: 提高磁盘 I/O 性能
3. **网络优化**: 使用高速网络连接
4. **缓存策略**: 配置适当的缓存

## 常用命令

```bash
# 查看运行中的容器
docker ps

# 查看所有容器（包括停止的）
docker ps -a

# 进入容器
docker exec -it <container_id> /bin/bash

# 清理未使用的资源
docker system prune -a

# 查看资源使用
docker stats
```
