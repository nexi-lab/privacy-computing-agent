package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

func RunCmd(cmd string) error {
	c := exec.Command("/bin/sh", "-c", cmd)
	output, err := c.CombinedOutput() // 等待命令结束，并返回 stdout/stderr
	if err != nil {
		log.Error("Command error:", cmd, err.Error())
		return err
	}
	log.Debugf("command %s Output: %s", cmd, string(output))
	return nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

func GetIp(domain string) string {
	ips, err := net.LookupIP(domain)
	if err != nil {
		fmt.Println("Failed to resolve:", err)
		return domain
	}

	log.Printf("IP addresses for %s:\n", domain)
	for _, ip := range ips {
		fmt.Println(ip.String())
		return ip.String()
	}
	return domain
}
func IsPortOpen(port string) bool {
	// 默认检查本地主机
	const host = "127.0.0.1"
	const timeout = 2 * time.Second // 设置连接超时时间为 2 秒
	address := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		fmt.Printf("DEBUG: Connection to %s failed: %v\n", address, err)
		return false
	}
	if conn != nil {
		conn.Close()
		return true
	}
	return false
}

func CopyFile(src, dst string) (written int64, err error) {
	sourceFile, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("failed to open source file %s: %w", src, err)
	}
	defer sourceFile.Close() // 确保源文件句柄被关闭

	destinationFile, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("failed to create destination file %s: %w", dst, err)
	}
	defer destinationFile.Close() // 确保目标文件句柄被关闭

	// 3. 使用 io.Copy 复制数据
	written, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return written, fmt.Errorf("failed to copy data: %w", err)
	}
	return written, nil
}
func replaceInFile(path string, replacements map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := string(data)
	for old, newVal := range replacements {
		content = strings.ReplaceAll(content, old, newVal)
	}
	return os.WriteFile(path, []byte(content), 0644)
}
