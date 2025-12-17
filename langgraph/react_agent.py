#!/usr/bin/env python3
"""Simple ReAct Agent using LangGraph's Prebuilt create_react_agent.

This example demonstrates how to use LangGraph's prebuilt create_react_agent
function to quickly build a ReAct agent with Nexus filesystem integration.

Authentication:
    API keys are REQUIRED via metadata.x_auth: "Bearer <token>"
    Frontend automatically passes the authenticated user's API key in request metadata.
    Each tool extracts and uses the token to create an authenticated RemoteNexusFS instance.

Requirements:
    pip install langgraph langchain-anthropic

Usage from Frontend (HTTP):
    POST http://localhost:2024/runs/stream
    {
        "assistant_id": "agent",
        "input": {
            "messages": [{"role": "user", "content": "Find all Python files"}]
        },
        "metadata": {
            "x_auth": "Bearer sk-your-api-key-here",
            "user_id": "user-123",
            "tenant_id": "tenant-123",
            "opened_file_path": "/workspace/admin/script.py"  // Optional: currently opened file
        }
    }

    Note: The frontend automatically includes x_auth and opened_file_path in metadata when user is logged in.
"""

import os

from langchain_openai import ChatOpenAI
from langchain_core.messages import SystemMessage
from langchain_core.runnables import RunnableConfig, RunnableLambda
from langgraph.prebuilt import create_react_agent
from nexus_tools import get_nexus_tools

tools = get_nexus_tools()

OPENAI_API_KEY = os.getenv("OPENAI_API_KEY")
OPENAI_BASE_URL = os.getenv("OPENAI_API_BASE")
OPENAI_MODEL = os.getenv("OPENAI_MODEL")

llm = ChatOpenAI(
    model=OPENAI_MODEL,
    max_tokens=10000,
    api_key=OPENAI_API_KEY,
    base_url=OPENAI_BASE_URL,
    temperature=0,
)

SYSTEM_PROMPT = """你是一个隐私计算任务助手，负责协调两个用户（任务发起方 Initiator 与 协作方 Partner）
共同完成隐私求交与联合查询任务。

# 角色判断
    - 发起方与协作方都使用同一份 Prompt,并根据调用上下文自动识别自身身份

# 行为规范,特别注意
    - 执行的每一个步骤，需要反馈给用户正在执行的步骤以及结果
    - 中间步骤失败，需要反馈到客户说明原因
    - {} 为变量取值,不要自己定义,使用下面定义的变量

# Initiator规范
    - 必须协调完整流程
    - 协调 Partner 时候追加 (*你是协作方,请协作完成*), 用于Partner更好的角色判断
    - 调用 send_to_partner_agent 明确指定需要使用的 tool以及参数
    - Partner tool操作都需要通过 send_to_partner_agent 包装

# Partner规范
    - 直接使用 Initiator 明确指定的工具,参数可推理
    - 不要主动推理/猜测，需要配合 Initiator 提供的 tool 操作

# 特别注意,重要变量
    - 本方用户 user = 自动推理
    - 协作方用户 partner_user = 自动推理
    - 定位完成数据集后 nexus_fs_file = 自动推理
    - 容器名称为 container_name=tsql_{user}
    - 本方公钥 public_key  = 自动推理
    - 协作方公钥 partner_public_key  = 自动推理
    - 隐私计算sql写入 runsql  = 自动推理
    - 隐私计算的 task_json  = 推理的 task JSON

# 总体流程(Initiator 驱动)
    以下步骤不允许失败,失败需要中断流程,返回原因,等待用户确认后继续
    Initiator  按照指定步骤点用工具完成任务
    Partner 按照指定步骤点用工具 [重要:tools 使用 send_to_partner_agent 包装后调用 Partner]
    整个任务流程由 Initiator 严格管理，步骤如下：

1. 定位双方数据集文件(仅 CSV),获取表头,协作方数据集定位不要使用本方 glob_files,必须send_to_partner_agent 包装
    1.1. Initiator -> glob_files -> read_csv_header -> create_docker_container
    1.2. Partner -> glob_files -> read_csv_header -> create_docker_container
    1.3  双方交换公钥信息到对方，并且各自写入自己的 {task_json}

2. 生成求交 SQL(自动推断)
   - 自动规则：
       a. 若用户指定,并且双方数据集存在 -> 使用此列
       b. 若双方 CSV 存在完全相同列名 → 默认选择它为求交列  
       c. 若无完全匹配 → Initiator 提示用户手动选择
   - 基础 SQL 模板：
       SELECT {用户}.{交集列} FROM {用户} INNER JOIN {协作方用户} ON {用户}.{交集列} = {协作方用户}.{交集列} where ...;

3. 生成双方 Task JSON
   - 特别注意: 只有 Initiator 写入生成的sql到runsql,Partner 的 runsql 为空
   - columns 数组,所有列给 {partner_user} 开放 PLAINTEXT_AFTER_JOIN 权限

   Task JSON:
    {
        "user": "{user}",
        "data":"{nexus_fs_file}",
        "columns": [
            {
                "column": "{column}",
                "type": "string",
                "permissions": [
                    {
                        "user": "{partner_user}",
                        "permission": "PLAINTEXT_AFTER_JOIN"
                    }
                ]
            }
        ],
        "userkey": "{public_key}",
        "userurl": "http://tsql_{user}:8081",
        "engineURL": "tsql_{user}:8003",
        "party": {
            "user": "{partner_user}",
            "pubkey": "{partner_public_key}",
            "partyURL": "http://tsql_{partner_user}:8081",
        },
        "runsql": "{runsql}"
    }

4. 向用户确认 {task_json} 是否正确
  - 用户确认后继续后面步骤

5. 启动隐私计算
  Partner -> start_privacy_run 参数 {container_name} {task_json}
  Initiator -> start_privacy_run  参数 {container_name} {task_json}

6. 等待隐私计算
    1. Initiator  
     1.1 使用 query_log 检查日志,分析隐私计算是否完成,完成后提取 上传至 nexusfs 内文件路径
     1.2 使用 glob_files 检测 nexusfs 文件系统文件是否存在
"""

# 7. **停止容器（双方）**
#     - Initiator -> stop_and_remove_container
#     - Partner  -> stop_and_remove_container

def build_prompt(state: dict, config: RunnableConfig) -> list:
    """Build prompt with optional opened file context from metadata.

    This function is called before each LLM invocation and can access
    the config which includes metadata from the frontend.
    """
    # Extract opened_file_path from metadata
    metadata = config.get("metadata", {})
    opened_file_path = metadata.get("opened_file_path")

    # Build system prompt with optional context
    system_content = SYSTEM_PROMPT
    if opened_file_path:
        system_content += f"""
## Current file
The user currently has the following file open in their editor:
**{opened_file_path}**
"""
    # Return system message + user messages
    return [SystemMessage(content=system_content)] + state["messages"]

# Create a runnable that wraps the prompt builder
prompt_runnable = RunnableLambda(build_prompt)

# Create prebuilt ReAct agent with dynamic prompt
agent = create_react_agent(
    model=llm,
    tools=tools,
    prompt=prompt_runnable,
)

if __name__ == "__main__":
    # Example usage - Note: requires NEXUS_API_KEY to be set for testing
    import sys

    api_key = os.getenv("NEXUS_API_KEY")
    if not api_key:
        print("Error: NEXUS_API_KEY environment variable is required for testing")
        print("Usage: NEXUS_API_KEY=your-key python react_agent.py")
        sys.exit(1)

    print("Testing ReAct agent...")

    # Test with opened file context
    result = agent.invoke(
        {"messages": [{"role": "user", "content": "What does this file do?"}]},
        config={
            "metadata": {
                "x_auth": f"Bearer {api_key}",
                "opened_file_path": "/workspace/admin/test.py",  # Optional: simulates opened file
            }
        },
    )
    print(result)
