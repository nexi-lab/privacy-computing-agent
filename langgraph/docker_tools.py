import os
import io
import socket
import tarfile
from typing import Dict, Any, Optional, List
import docker


class DockerTools:
    """Docker 操作工具类"""

    def __init__(self, socket_path: str = "/var/run/docker.sock"):
        """
        初始化 Docker 客户端
        """
        try:
            self.client = docker.DockerClient(base_url=f'unix://{socket_path}')
            self.client.ping()
        except Exception as e:
            raise ConnectionError(f"无法连接到 Docker: {e}")

    def create_docker_container(
        self,
        container_name: str,
        image: str = "tsql:latest",
        command: Optional[List[str]] = None,
        ports: Optional[Dict[str, int]] = None,
        environment: Optional[Dict[str, str]] = None
    ) -> Dict[str, Any]:
        """
        使用默认镜像创建并启动容器（tsql:latest）

        Args:
            container_name: 名称
            image: 镜像名称
            command: 启动命令
            ports: 端口映射
            environment: 环境变量

        Returns:
            创建结果字典
        """
        try:
            # 若已存在同名容器，先删除
            try:
                exist = self.client.containers.get(container_name)
                exist.remove(force=True)
            except docker.errors.NotFound:
                pass
            
            current = self.client.containers.get(socket.gethostname())
            current_network = list(current.attrs["NetworkSettings"]["Networks"].keys())[0]
            container = self.client.containers.run(
                image=image,
                name=container_name,
                command=command,
                ports=ports,
                environment=environment,
                detach=True,
                auto_remove=False,
                network=current_network
            )

            return {
                "success": True,
                "container_id": container.id,
                "container_name": container.name,
                "image": image,
                "status": "created and started",
            }

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
                "message": f"容器创建失败: {e}"
            }

    # ====================================================
    # 停止容器（根据容器名称）
    # ====================================================
    def stop_container(self, container_name: str) -> Dict[str, Any]:
        """
        停止容器
        """
        try:
            container = self.client.containers.get(container_name)
            container.stop()

            return {
                "success": True,
                "container": container_name,
                "message": "容器已停止"
            }
        except Exception as e:
            return {"success": False, "error": str(e)}

    # ====================================================
    # 删除容器（根据容器名称）
    # ====================================================
    def remove_container(self, container_name: str, force: bool = False) -> Dict[str, Any]:
        """
        删除容器
        """
        try:
            container = self.client.containers.get(container_name)
            container.remove(force=force)

            return {
                "success": True,
                "container": container_name,
                "message": "容器已删除"
            }
        except Exception as e:
            return {"success": False, "error": str(e)}

    # ====================================================
    # 上传文件
    # ====================================================
    def upload_file_to_container(
        self,
        container_id: str,
        local_file_path: str,
        container_path: str
    ) -> Dict[str, Any]:
        """
        上传文件到容器
        """
        try:
            container = self.client.containers.get(container_id)

            if not os.path.exists(local_file_path):
                return {"success": False, "error": f"本地文件不存在: {local_file_path}"}

            tar_stream = io.BytesIO()
            with tarfile.open(fileobj=tar_stream, mode='w') as tar:
                tar.add(local_file_path, arcname=os.path.basename(container_path))
            tar_stream.seek(0)

            container.put_archive(
                path=os.path.dirname(container_path) or "/",
                data=tar_stream
            )

            return {
                "success": True,
                "message": f"文件已上传到容器 {container_id}:{container_path}",
                "container_id": container_id,
                "container_path": container_path
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    # ====================================================
    # 容器执行命令
    # ====================================================
    def execute_command_in_container(
        self,
        container_id: str,
        command: str,
        workdir: Optional[str] = None,
        environment: Optional[Dict[str, str]] = None
    ) -> Dict[str, Any]:
        """
        在容器中执行命令
        """
        try:
            container = self.client.containers.get(container_id)

            exec_result = container.exec_run(
                cmd=command,
                workdir=workdir,
                environment=environment,
                stdout=True,
                stderr=True
            )

            return {
                "success": exec_result.exit_code == 0,
                "exit_code": exec_result.exit_code,
                "output": exec_result.output.decode('utf-8') if exec_result.output else "",
            }

        except Exception as e:
            return {"success": False, "error": str(e)}

    # ====================================================
    # 获取文件
    # ====================================================
    def get_file_from_container(
        self,
        container_id: str,
        container_file_path: str,
        local_save_path: str
    ) -> Dict[str, Any]:
        """
        从容器获取文件
        """
        try:
            container = self.client.containers.get(container_id)

            bits, stat = container.get_archive(container_file_path)

            file_obj = io.BytesIO()
            for chunk in bits:
                file_obj.write(chunk)
            file_obj.seek(0)

            with tarfile.open(fileobj=file_obj, mode='r') as tar:
                members = tar.getmembers()
                if not members:
                    return {"success": False, "error": "文件不存在"}

                file_member = members[0]
                file_content = tar.extractfile(file_member).read()

                if local_save_path:
                    os.makedirs(os.path.dirname(local_save_path), exist_ok=True)
                    with open(local_save_path, 'wb') as f:
                        f.write(file_content)
                    return {
                        "success": True,
                        "local_path": local_save_path
                    }

                return {
                    "success": True,
                    "file_size": len(file_content),
                }

        except Exception as e:
            return {"success": False, "error": str(e)}
        
    def check_file_exists_in_container(
        self,
        container_id: str,
        container_file_path: str
    ) -> Dict[str, Any]:
        """
        检查容器内文件是否存在（不读取内容）

        Args:
            container_id (str): 容器 ID 或名称
            container_file_path (str): 容器内部文件路径

        Returns:
            {
                "success": True,
                "exists": True/False
            }
        """
        try:
            container = self.client.containers.get(container_id)

            # 使用 test 命令检查是否存在
            exit_code, _ = container.exec_run(f"test -f {container_file_path}")

            return {
                "success": True,
                "exists": (exit_code == 0),
                "path": container_file_path
            }

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
                "path": container_file_path
            }

    # ====================================================
    # 列出所有容器
    # ====================================================
    def list_containers(self) -> List[Dict[str, Any]]:
        try:
            containers = self.client.containers.list(all=True)
            return [
                {
                    "id": c.id,
                    "name": c.name,
                    "status": c.status,
                    "image": c.image.tags[0] if c.image.tags else c.image.id
                }
                for c in containers
            ]
        except Exception as e:
            return [{"error": str(e)}]
