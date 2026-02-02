# Privacy Computing Agent (LangGraph)

基于 LangGraph 的隐私计算智能代理，负责协调多方参与者完成隐私求交和联邦 SQL 查询任务。

## 功能概述

Privacy Computing Agent 是系统的核心智能组件，使用 ReAct (Reasoning + Acting) 模式实现：

1. **智能任务协调**: 自动识别角色（发起方 Initiator / 协作方 Partner）
2. **流程编排**: 协调完整的隐私计算工作流
3. **文件系统操作**: 通过 Nexus Tools 管理数据集
4. **容器管理**: 创建和管理隐私计算容器
5. **多方通信**: 协调发起方和协作方的交互

## 架构设计

```
┌─────────────────────────────────────┐
│         LangGraph Agent             │
│                                     │
│  ┌──────────────────────────────┐  │
│  │   ReAct Agent (react_agent.py)│  │
│  │   - 推理与行动循环            │  │
│  │   - 角色识别                  │  │
│  │   - 流程控制                  │  │
│  └──────────┬───────────────────┘  │
│             │                       │
│  ┌──────────┴───────────────────┐  │
│  │        Tools                  │  │
│  │                               │  │
│  │  ┌─────────────────────────┐ │  │
│  │  │  Nexus Tools            │ │  │
│  │  │  - glob_files           │ │  │
│  │  │  - read_csv_header      │ │  │
│  │  │  - send_to_partner      │ │  │
│  │  └─────────────────────────┘ │  │
│  │                               │  │
│  │  ┌─────────────────────────┐ │  │
│  │  │  Docker Tools           │ │  │
│  │  │  - create_container     │ │  │
│  │  │  - start_privacy_run    │ │  │
│  │  │  - query_log            │ │  │
│  │  └─────────────────────────┘ │  │
│  └───────────────────────────────┘  │
└─────────────────────────────────────┘
```

## 核心组件

### 1. ReAct Agent (react_agent.py)

基于 LangGraph 的 `create_react_agent` 实现的智能代理。

**主要特性**:
- 使用 OpenAI 兼容的 LLM（支持 DeepSeek、Qwen 等）
- 动态 Prompt 构建，支持上下文注入
- 工具调用与推理循环
- 流式响应支持

**System Prompt 设计**:

Agent 使用统一的 Prompt 同时支持发起方和协作方角色：

```python
SYSTEM_PROMPT = """你是一个隐私计算任务助手，负责协调两个用户（任务发起方 Initiator 与 协作方 Partner）
共同完成隐私求交与联合查询任务。

# 角色判断
- 发起方与协作方都使用同一份 Prompt,并根据调用上下文自动识别自身身份

# 行为规范
- 执行的每一个步骤，需要反馈给用户正在执行的步骤以及结果
- 中间步骤失败，需要反馈到客户说明原因

# Initiator规范
- 必须协调完整流程
- 调用 send_to_partner_agent 明确指定需要使用的 tool以及参数
- Partner tool操作都需要通过 send_to_partner_agent 包装

# Partner规范
- 直接使用 Initiator 明确指定的工具,参数可推理
- 不要主动推理/猜测，需要配合 Initiator 提供的 tool 操作
"""
```

### 2. Nexus Tools (nexus_tools.py)

与 Nexus 文件系统交互的工具集。

**可用工具**:

#### glob_files
查找文件：
```python
@tool
def glob_files(pattern: str, config: RunnableConfig, path: str = "/") -> str:
    """Find files by name pattern.

    Args:
        pattern: Glob pattern (e.g., "*.py", "**/*.csv")
        path: Directory to search (default "/")
    """
```

#### read_csv_header
读取 CSV 表头：
```python
@tool
def read_csv_header(file_path: str, config: RunnableConfig) -> str:
    """Read CSV file header (first row).

    Args:
        file_path: Path to CSV file
    """
```

#### send_to_partner_agent
向协作方发送请求：
```python
@tool
def send_to_partner_agent(
    message: str,
    tool_name: str,
    tool_args: dict,
    config: RunnableConfig
) -> str:
    """Send request to partner agent.

    Args:
        message: Message to partner
        tool_name: Tool to invoke on partner side
        tool_args: Tool arguments
    """
```

#### get_public_key
获取容器公钥：
```python
@tool
def get_public_key(container_name: str, config: RunnableConfig) -> str:
    """Get public key from privacy computing container.

    Args:
        container_name: Container name (e.g., "tsql_alice")
    """
```

### 3. Docker Tools (docker_tools.py)

Docker 容器管理工具类。

**主要方法**:

#### create_docker_container
创建隐私计算容器：
```python
def create_docker_container(
    container_name: str,
    image: str = "tsql:latest",
    command: Optional[List[str]] = None,
    ports: Optional[Dict[str, int]] = None,
    environment: Optional[Dict[str, str]] = None
) -> Dict[str, Any]
```

#### start_privacy_run
启动隐私计算任务：
```python
def start_privacy_run(
    container_name: str,
    task_json: str
) -> Dict[str, Any]
```

#### query_log
查询容器日志：
```python
def query_log(
    container_name: str,
    tail: int = 100
) -> Dict[str, Any]
```

## 工作流程

### 完整的隐私计算流程

Agent 按照以下步骤协调任务：

```
1. 数据集定位
   ├─ Initiator: glob_files → read_csv_header
   └─ Partner: glob_files → read_csv_header (via send_to_partner_agent)

2. 容器创建
   ├─ Initiator: create_docker_container
   └─ Partner: create_docker_container (via send_to_partner_agent)

3. 公钥交换
   ├─ Initiator: get_public_key → send to Partner
   └─ Partner: get_public_key → send to Initiator

4. SQL 生成
   └─ Initiator: 根据表头自动生成联邦 SQL

5. Task JSON 生成
   └─ Initiator: 生成双方的配置文件

6. 用户确认
   └─ Initiator: 展示配置，等待用户确认

7. 启动计算
   ├─ Partner: start_privacy_run (via send_to_partner_agent)
   └─ Initiator: start_privacy_run

8. 等待结果
   └─ Initiator: query_log → glob_files (检查结果文件)
```

### Task JSON 格式

```json
{
  "user": "alice",
  "data": "/workspace/alice/data.csv",
  "columns": [
    {
      "column": "id",
      "type": "string",
      "permissions": [
        {
          "user": "bob",
          "permission": "PLAINTEXT_AFTER_JOIN"
        }
      ]
    }
  ],
  "userkey": "MCowBQYDK2VwAyEA...",
  "userurl": "http://tsql_alice:8081",
  "engineURL": "tsql_alice:8003",
  "party": {
    "user": "bob",
    "pubkey": "MCowBQYDK2VwAyEA...",
    "partyURL": "http://tsql_bob:8081"
  },
  "runsql": "SELECT alice.id FROM alice INNER JOIN bob ON alice.id = bob.id"
}
```

## 配置说明

### 环境变量

```bash
# OpenAI 兼容 API 配置
OPENAI_API_KEY=sk-your-api-key
OPENAI_API_BASE=https://api.openai.com/v1
OPENAI_MODEL=gpt-4

# Nexus 服务器（通过 metadata 传递）
# nexus_server_url: http://nexus:8080
```

### langgraph.json

LangGraph 服务配置：

```json
{
  "dependencies": ["."],
  "graphs": {
    "agent": "./react_agent.py:agent"
  },
  "env": ".env"
}
```

### pyproject.toml

Python 依赖配置：

```toml
[project]
name = "privacy-computing-agent"
version = "0.1.0"
dependencies = [
    "langgraph",
    "langchain-openai",
    "nexus-remote-fs",
    "docker",
    "cryptography"
]
```

## 本地开发

### 前置要求

- Python 3.11+
- Docker (用于容器管理)
- Nexus 服务器访问权限

### 安装依赖

```bash
cd langgraph
pip install -e .
```

### 配置环境

```bash
export OPENAI_API_KEY=sk-your-api-key
export OPENAI_API_BASE=https://api.openai.com/v1
export OPENAI_MODEL=gpt-4
export NEXUS_API_KEY=sk-nexus-key
```

### 运行测试

```bash
python react_agent.py
```

### 使用 LangGraph CLI

```bash
# 安装 LangGraph CLI
pip install langgraph-cli

# 启动开发服务器
langgraph dev

# 生产部署
langgraph up
```

## API 使用

### HTTP 请求示例

```bash
curl -X POST http://localhost:8123/runs/stream \
  -H "Content-Type: application/json" \
  -d '{
    "assistant_id": "agent",
    "input": {
      "messages": [
        {
          "role": "user",
          "content": "帮我和 bob 完成隐私求交，我的数据集是 /workspace/alice/data.csv"
        }
      ]
    },
    "metadata": {
      "x_auth": "Bearer sk-alice-key",
      "user_id": "alice",
      "target_user_id": "bob",
      "nexus_server_url": "http://nexus:8080"
    }
  }'
```

### Python SDK 示例

```python
from langgraph_sdk import get_client

client = get_client(url="http://localhost:8123")

# 创建线程
thread = await client.threads.create()

# 发送消息
async for chunk in client.runs.stream(
    thread["thread_id"],
    "agent",
    input={
        "messages": [
            {"role": "user", "content": "查找 CSV 文件"}
        ]
    },
    metadata={
        "x_auth": "Bearer sk-key",
        "user_id": "alice",
        "nexus_server_url": "http://nexus:8080"
    }
):
    print(chunk)
```

## Docker 部署

### 构建镜像

```bash
docker build -t tsql-langgraph:latest .
```

### Dockerfile 说明

```dockerfile
FROM python:3.11-slim

WORKDIR /app

# 安装 Docker CLI (用于容器管理)
RUN apt-get update && apt-get install -y docker.io

# 安装 Python 依赖
COPY pyproject.toml .
RUN pip install -e .

# 复制代码
COPY . .

# 启动 LangGraph 服务
CMD ["langgraph", "up"]
```

### 运行容器

```bash
docker run -d \
  --name tsql-agent \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e OPENAI_API_KEY=sk-key \
  -e OPENAI_API_BASE=https://api.openai.com/v1 \
  -e OPENAI_MODEL=gpt-4 \
  tsql-langgraph:latest
```

**重要**: 需要挂载 Docker socket 以支持容器管理功能。

## 故障排查

### 问题 1: 无法连接到 Nexus

**症状**: 工具调用失败，提示 "Missing x_auth in metadata"

**解决方案**:
- 确认请求 metadata 中包含 `x_auth` 字段
- 验证 API Key 格式: `Bearer sk-xxx`
- 检查 `nexus_server_url` 配置

### 问题 2: Docker 容器创建失败

**症状**: create_docker_container 返回错误

**解决方案**:
```bash
# 检查 Docker socket 权限
ls -l /var/run/docker.sock

# 确认镜像存在
docker images | grep tsql

# 检查网络连通性
docker network ls
```

### 问题 3: Partner Agent 通信失败

**症状**: send_to_partner_agent 超时或失败

**解决方案**:
- 检查 Partner 的 Agent Proxy 是否运行
- 验证 `target_user_id` 是否已注册
- 查看 Agent Proxy 日志

### 问题 4: LLM 响应异常

**症状**: Agent 行为不符合预期

**解决方案**:
- 检查 System Prompt 是否正确加载
- 验证 LLM 配置（API Key、Base URL、Model）
- 增加日志级别查看推理过程

## 性能优化

1. **LLM 选择**:
   - 简单任务使用 DeepSeek-V3（性价比高）
   - 复杂推理使用 GPT-4

2. **并发处理**:
   - 使用异步工具调用
   - 并行处理双方操作

3. **缓存策略**:
   - 缓存 Partner Thread ID（60分钟 TTL）
   - 缓存公钥信息

## 扩展开发

### 添加新工具

```python
@tool
def my_custom_tool(arg1: str, config: RunnableConfig) -> str:
    """Tool description.

    Args:
        arg1: Argument description
    """
    # 获取认证信息
    nx = _get_nexus_client(config)

    # 实现工具逻辑
    result = do_something(arg1)

    return result

# 注册工具
def get_nexus_tools():
    return [
        glob_files,
        read_csv_header,
        my_custom_tool,  # 添加新工具
        # ...
    ]
```

### 自定义 Prompt

修改 `react_agent.py` 中的 `SYSTEM_PROMPT` 变量来自定义 Agent 行为。

## 未来改进

- [ ] 支持更多隐私计算协议（PSI-CA、PIR 等）
- [ ] 添加任务状态持久化
- [ ] 支持断点续传
- [ ] 优化错误处理和重试机制
- [ ] 添加监控和可观测性
- [ ] 支持多语言（当前仅中文）
