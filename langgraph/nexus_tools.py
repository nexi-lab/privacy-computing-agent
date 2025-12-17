import os
import shlex

from langchain_core.runnables import RunnableConfig
from langchain_core.tools import tool
from nexus.remote import RemoteNexusFS
from docker_tools import DockerTools
from typing import Tuple
from cryptography.hazmat.primitives.asymmetric import ed25519
from cryptography.hazmat.primitives import serialization
import base64
import logging
import json
from langgraph_sdk import get_client
import asyncio
import tempfile
import time
from typing import Dict, Any
import requests

PARTNER_THREADS = {}
CACHE_TTL = 60*60  # 秒，例如 60 分钟
container_private_file="/home/user/config/ed25519key.pem"

logging.basicConfig(level=logging.DEBUG, format='%(asctime)s - %(levelname)s - %(message)s')

def get_nexus_tools():
    """
    Create LangGraph tools that connect to Nexus server with per-request authentication.

    Args:
        server_url: Nexus server URL (e.g., "http://localhost:8080" or ngrok URL)

    Returns:
        List of LangGraph tool functions that require x_auth in metadata

    Usage:
        tools = get_nexus_tools("http://localhost:8080")
        agent = create_react_agent(model=llm, tools=tools)

        # Frontend passes API key in metadata:
        result = agent.invoke(
            {"messages": [{"role": "user", "content": "Find Python files"}]},
            metadata={"x_auth": "Bearer sk-your-api-key"}
        )
    """

    docker_tools = DockerTools()

    def _get_nexus_client(config: RunnableConfig) -> RemoteNexusFS:
        """Create authenticated RemoteNexusFS from config.

        Requires authentication via metadata.x_auth: "Bearer <token>"
        """
        # Get API key from metadata.x_auth (required)
        metadata = config.get("metadata", {})
        x_auth = metadata.get("x_auth", "")
        server_url = metadata.get("nexus_server_url", "")

        if not x_auth:
            raise ValueError(
                "Missing x_auth in metadata. "
                "Frontend must pass API key via metadata: {'x_auth': 'Bearer <token>'}"
            )

        # Strip "Bearer " prefix if present
        api_key = x_auth.removeprefix("Bearer ").strip()

        if not api_key:
            raise ValueError("Invalid x_auth format. Expected 'Bearer <token>', got: " + x_auth)

        return RemoteNexusFS(server_url=server_url, api_key=api_key)

    @tool
    def glob_files(pattern: str, config: RunnableConfig, path: str = "/") -> str:
        """Find files by name pattern.

        Args:
            pattern: Glob pattern (e.g., "*.py", "**/*.csv", "test_*.csv")
            path: Directory to search (default "/")

        Examples: glob_files("*.csv", "/workspace"), glob_files("**/*.csv")
        """
        try:
            # Get authenticated client
            nx = _get_nexus_client(config)

            files = nx.glob(pattern, path)

            if not files:
                return f"No files found matching pattern '{pattern}' in {path}"

            # Format results
            output_lines = [f"Found {len(files)} files matching '{pattern}' in {path}:\n"]
            output_lines.extend(f"  {file}" for file in files[:100])  # Limit to first 100

            if len(files) > 100:
                output_lines.append(f"\n... and {len(files) - 100} more files")

            return "\n".join(output_lines)

        except Exception as e:
            return f"Error finding files: {str(e)}"

    @tool
    def check_container_file(
        container_id: str,
        container_file_path: str
    ) -> str:
        """
        检查 Docker 容器内的文件是否存在。
        不会读取文件内容，仅检测文件是否存在且为常规文件。

        Args:
            container_id (str):
                Docker 容器 ID 或名称。

            container_file_path (str):
                容器内部文件路径，例如 "/app/output/result.csv"。

        Returns:
            JSON 字符串格式：
            成功：
            {
                "success": true,
                "exists": true,
                "path": "<container_file_path>"
            }

            文件不存在：
            {
                "success": true,
                "exists": false,
                "path": "<container_file_path>"
            }

            错误：
            {
                "success": false,
                "error": "<error message>",
                "path": "<container_file_path>"
            }

        Examples:
            check_container_file("ctr123", "/output/result.csv")
        """
        try:
            result = docker_tools.check_file_exists_in_container(
                container_id=container_id,
                container_file_path=container_file_path
            )
            return json.dumps(result, ensure_ascii=False, indent=2)

        except Exception as e:
            return json.dumps({
                "success": False,
                "error": str(e),
                "path": container_file_path
            }, ensure_ascii=False, indent=2)

    @tool
    def query_log(container_name: str) ->str:
        """
        在 Docker 容器中查询隐私计算的日志
        
        Args:
            container_name: 容器 ID 或名称
            
        Returns:
            隐私计算日志
        """
        
        result = docker_tools.execute_command_in_container(container_name,
                                                           "cat /home/user/tsqlctl.log")
        if result["success"]:
            return result["output"]
        return result["error"]

    @tool
    def start_privacy_run(
        container_name: str,
        task_json: Dict[str, Any]
    ) -> str:
        """
        Start a privacy computation task.

        Args:
            container_name: service/container name, e.g. privacy-service
            task_json: request body for /api/privacy/run
        Returns:
            task_id if success, otherwise error message
        """

        url = f"http://{container_name}:8000/api/privacy/run"

        try:
            resp = requests.post(
                url,
                json=task_json,
                timeout=10,
            )
        except requests.RequestException as e:
            return f"privacy service request failed: {e}"

        if resp.status_code != 200:
            return f"privacy service error: status={resp.status_code}, body={resp.text}"

        try:
            data = resp.json()
        except json.JSONDecodeError:
            return f"invalid json response: {resp.text}"

        task_id = data.get("task_id")
        if not task_id:
            return f"missing task_id in response: {data}"

        return task_id

    @tool
    def read_csv_header(path: str, config: RunnableConfig) -> str:
        """
        Read the header row (first line) of a CSV file stored in NexusFS.

        This tool loads a CSV file from NexusFS and returns the first line split by
        commas. The file content is decoded as UTF-8. If the file is empty or the
        first line is blank, an empty header list will be returned.

        Args:
            path (str):
                Full NexusFS file path, e.g. "/datasets/users.csv".
                Must refer to a readable file in NexusFS.
            config (RunnableConfig):
                Runtime configuration automatically provided by the agent for
                authentication and NexusFS access. Do not pass manually.

        Returns:
            str (JSON string):
                On success:
                {
                    "success": true,
                    "header": ["col1", "col2", ...],
                    "columns_count": <number of columns>
                }

                On failure:
                {
                    "success": false,
                    "error": "<error message>"
                }

        Behavior Notes:
            - Reads the entire file content into memory.
            - CSV is assumed to be comma-separated.
            - File is decoded as UTF-8; invalid bytes are ignored.
            - The header is taken as the first line before the first newline.
            - Whitespace around column names is stripped.
            - Does not validate CSV structure beyond the first line.

        Examples:
            read_csv_header("/data/train.csv")
            read_csv_header("/public/records/user_data.csv")
        """
        logging.info(f"read_csv_header: {path}")
        try:
            nx = _get_nexus_client(config)
            logging.info(f"nx: {nx}")
            content = nx.read(path)
            logging.info(f"content: {content}")
            if isinstance(content, bytes):
                content = content.decode("utf-8", errors="ignore")

            # First line only
            first_line = content.split("\n", 1)[0]
            header = [x.strip() for x in first_line.split(",") if x.strip()]

            return json.dumps({
                "success": True,
                "header": header,
                "columns_count": len(header)
            }, ensure_ascii=False)

        except Exception as e:
            return json.dumps({
                "success": False,
                "error": str(e)
            }, ensure_ascii=False)

    def generate() -> Tuple[str, str]:
        """
        生成 Ed25519 公私钥，并保存到文件。
        
        返回:
            (private_key_path, public_key_path)
        """

        # 生成 Ed25519 密钥对
        private_key = ed25519.Ed25519PrivateKey.generate()
        public_key = private_key.public_key()

        # 导出 PEM 私钥
        private_pem = private_key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        )

        # 导出 DER 格式的公钥
        public_der = public_key.public_bytes(
            encoding=serialization.Encoding.DER,
            format=serialization.PublicFormat.SubjectPublicKeyInfo,
        )

        fd, private_file = tempfile.mkstemp()
        with os.fdopen(fd, "wb") as f:
            f.write(private_pem)

        fd, public_file = tempfile.mkstemp()
        with os.fdopen(fd, "wb") as f:
            f.write(public_der)
        return private_file, public_file

    # 定义工具函数
    @tool
    def create_docker_container(container_name: str,config: RunnableConfig) -> str:
        """
        use default image create container
        Args:
            container_name: container name
            config (RunnableConfig):
                Runtime configuration automatically injected by the agent.
                Used internally to authenticate with NexusFS.
        Returns:
            create container result
            container public key
        """

        metadata = config.get("metadata", {})
        x_auth = metadata.get("x_auth", "")
        api_key = x_auth.removeprefix("Bearer ").strip()
        server_url = metadata.get("nexus_server_url", "")

        result = docker_tools.create_docker_container(
            container_name=container_name,
            environment={
                "NEXUS_SERVER_URL":server_url,
                "NEXUS_API_KEY":api_key
            }
        )
        if result["success"]==False:
            return json.dumps(result, indent=2, ensure_ascii=False)

        private_file,public_file=generate()

        result = docker_tools.upload_file_to_container(
            container_id=result["container_id"],
            local_file_path=private_file,
            container_path=container_private_file
        )
        if result["success"]==False:
            return json.dumps(result, indent=2, ensure_ascii=False)
        
        with open(public_file, "rb") as f:
            data = f.read()
        result["public_key"]=base64.b64encode(data).decode("ascii")

        return json.dumps(result, indent=2, ensure_ascii=False)
    
    @tool
    def stop_and_remove_container(container_name: str) -> str:
        """
        停止并且删除容器
        Args:
            container_name: 容器名称
        Returns:
            创建结果和网络地址信息
        """
        docker_tools.stop_container(
            container_name=container_name
        )
        result =  docker_tools.remove_container(
            container_name=container_name
        )
        return json.dumps(result, indent=2, ensure_ascii=False)
        
    async def _get_or_create_thread(client_url: str):
        now = time.time()

        # 1️⃣ 命中缓存且未过期
        cache = PARTNER_THREADS.get(client_url)
        if cache and cache["expire_at"] > now:
            return cache["client"], cache["thread_id"]

        # 2️⃣ 命中过期缓存 → 清理
        if cache:
            PARTNER_THREADS.pop(client_url, None)

        # 3️⃣ 创建新 client
        client = get_client(url=client_url)

        # 4️⃣ 创建新 thread
        thread = await client.threads.create()
        thread_id = thread["thread_id"]

        # 5️⃣ 写入缓存
        PARTNER_THREADS[client_url] = {
            "client": client,
            "thread_id": thread_id,
            "expire_at": now + CACHE_TTL,
        }
        return client, thread_id
        
    @tool
    async def send_to_partner_agent(target_user_id: str,text: str) -> str:
        """
        调用合作方 Agent,发送任务并等待返回内容。

        Args:
            target_user_id: 协作方用户名称 例如 bob,alice 等
            text: 要发送的内容

        Returns:
            对方 agent 回复的文本
        """
        async def _run():
            client, thread_id = await _get_or_create_thread("http://agent_proxy:2024")

            # 输入格式
            input_payload = {
                "messages": [
                    {"type": "human", "content": text}
                ]
            }

            metadata = {
                "target_user_id":target_user_id,
            }

            final_text = ""

            async for chunk in client.runs.stream(
                assistant_id="agent",
                thread_id=thread_id,
                metadata=metadata,
                input=input_payload
            ):
                for msg in chunk.data.get("messages", []):
                    if msg.get("type") == "ai":
                        final_text = msg.get("content", "")

            return final_text

        try:
            return await _run()
        except Exception as e:
            return f"Error calling partner agent: {e}"

    # Return all tools
    tools = [
        glob_files,
        check_container_file,
        read_csv_header,
        start_privacy_run,
        query_log,
        create_docker_container,
        stop_and_remove_container,
        send_to_partner_agent,
    ]

    return tools
