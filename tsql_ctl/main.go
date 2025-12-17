package main

import (
	"net/http"
	"os"
	"runtime/debug"

	_ "github.com/go-sql-driver/mysql"
	"github.com/secretflow/scql/pkg/util/brokerutil"
	log "github.com/sirupsen/logrus"
)

// 初始化函数：设置日志的输出目标和格式
func init() {
	brokerCommand = brokerutil.NewCommand("http://127.0.0.1:8080", 30)

	file, err := os.OpenFile("tsqlctl.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("无法打开日志文件:", err)
	}

	// 设置输出到多个 writer（终端 + 文件）
	log.SetOutput(file)

	log.SetLevel(log.DebugLevel)

	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("panic recovered: %v\nstacktrace:\n%s", r, debug.Stack())
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/privacy/run", runPrivacyHandler)

	log.Println("privacy service listening on :8000")
	log.Fatal(http.ListenAndServe(":8000", mux))
}
