package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

var resultFile = "/home/user/result.csv"
var DATAFILE = "/home/user/data.csv"
var dsn = "root:@tcp(127.0.0.1:3306)/engine?charset=utf8mb4&parseTime=True"

type RunPrivacyRequest struct {
	User    string `json:"user"`
	Data    string `json:"data"`
	Columns []struct {
		Column      string `json:"column"`
		Type        string `json:"type"`
		Permissions []struct {
			User       string `json:"user"`
			Permission string `json:"permission"`
		} `json:"permissions"`
	} `json:"columns"`

	UserKey   string `json:"userkey"`
	UserURL   string `json:"userurl"`
	EngineURL string `json:"engineURL"`

	Party struct {
		User     string `json:"user"`
		PubKey   string `json:"pubkey"`
		PartyURL string `json:"partyURL"`
	} `json:"party"`

	RunSQL string `json:"runsql"`
}

type RunPrivacyResponse struct {
	TaskID string `json:"task_id"`
	Status string `json:"status"`
}

func runPrivacyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RunPrivacyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	// ===== 基础参数校验（必要）=====
	if req.User == "" || req.Data == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	// ===== 生成任务 ID =====
	taskID := uuid.NewString()

	// ===== 异步启动隐私计算任务 =====
	go startPrivacyTask(taskID, &req)

	resp := RunPrivacyResponse{
		TaskID: taskID,
		Status: "submitted",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// 读取 task.json 替换网络变量
func DealTask(req *RunPrivacyRequest) {
	replaceInFile("/home/user/config/config.yml", map[string]string{
		"_NODE_NAME_":       req.User,
		"_NODE_ENGINE_URL_": req.EngineURL,
	})
	replaceInFile("/home/user/config/party_info.json", map[string]string{
		"_NODE_NAME_":        req.User,
		"_PARTY_NAME_":       req.Party.User,
		"_PARTY_PUBKEY_":     req.Party.PubKey,
		"_PARTY_SERVER_URL_": req.Party.PartyURL,
		"_NODE_SERVER_URL_":  req.UserURL,
		"_NODE_PUBKEY_":      req.UserKey,
	})
}

func checkDataset(req *RunPrivacyRequest) error {
	db, err := GetDB(dsn)
	if err != nil {
		return err
	}

	f, err := os.Open(DATAFILE)
	if err != nil {
		log.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	// 创建 CSV reader
	reader := csv.NewReader(f)

	// 读取第一行（表头）
	header, err := reader.Read()
	if err != nil {
		log.Fatalf("failed to read header: %v", err)
	}

	err = ExecSQL(db, fmt.Sprintf("drop table IF EXISTS %s", req.User))
	if err != nil {
		return err
	}
	create_table_sql := fmt.Sprintf("CREATE TABLE %s (", req.User)
	for n, column := range header {
		if n > 0 {
			create_table_sql += ","
		}
		create_table_sql += fmt.Sprintf("%s varchar(512)", column)
	}
	create_table_sql += ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;"

	err = ExecSQL(db, create_table_sql)
	if err != nil {
		return err
	}

	err = ExecSQL(db, fmt.Sprintf("delete from %s", req.User))
	if err != nil {
		return err
	}

	load_data_sql := fmt.Sprintf(`
		LOAD DATA INFILE '%s' INTO TABLE %s
		FIELDS TERMINATED BY ','      
		ENCLOSED BY '"'               
		LINES TERMINATED BY '\n'      
		IGNORE 1 LINES                
		(%s)`, DATAFILE, req.User, strings.Join(header, ","))

	return ExecSQL(db, load_data_sql)
}

func startPrivacyTask(taskID string, req *RunPrivacyRequest) {
	log.Printf("[task=%s] start privacy compute", taskID)
	log.Printf("[task=%s] input data: %s", taskID, req.Data)
	log.Printf("[task=%s] run sql: %s", taskID, req.RunSQL)
	log.Printf("[task=%s] engine url: %s", taskID, req.EngineURL)

	DealTask(req)

	for {
		err := RunCmd("supervisorctl restart broker")
		if err != nil {
			log.Debugf("[task=%s] err:%s", taskID, err.Error())
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	i := 60
	for i > 0 {
		if IsPortOpen("3306") {
			log.Debugf("[task=%s] 3306 is ok", taskID)
			break
		}
		time.Sleep(time.Second)
		i--
	}

	// download data.csv
	client := NewClient(os.Getenv("NEXUS_SERVER_URL"), os.Getenv("NEXUS_API_KEY"))
	data, err := client.ReadFile(context.Background(), req.Data)
	if err != nil {
		log.Errorf("[task=%s] err:%s", taskID, err.Error())
		return
	}
	err = os.WriteFile(DATAFILE, data, os.ModePerm)
	if err != nil {
		log.Errorf("[task=%s] err:%s", taskID, err.Error())
		return
	}
	err = checkDataset(req)
	if err != nil {
		log.Errorf("[task=%s] err:%s", taskID, err.Error())
		return
	}

	i = 60
	for i > 0 {
		if IsPortOpen("8080") && IsPortOpen("8081") && IsPortOpen("8003") {
			log.Debugf("[task=%s] 8080/8003 is ok", taskID)
			break
		}
		time.Sleep(time.Second)
		i--
	}

	if req.RunSQL != "" {
		log.Debugf("[task=%s] RunSQL--->ok", taskID)
		err := createProject()
		if err != nil {
			if !strings.Contains(err.Error(), "project tsql already exists") {
				log.Errorf("[task=%s] createProject err %s", taskID, err.Error())
				return
			}
		}
		// invite member
		for {
			err := inviteMember(req.Party.User)
			if err != nil {
				if strings.Contains(err.Error(), "project already contains invitee") {
					break
				}
				log.Debugf("[task=%s] inviteMember waiting... %s", taskID, err.Error())
				time.Sleep(3 * time.Second)
			} else {
				break
			}
		}
		// wait for joined
		for {
			joined, err := ProjectMemberJoined(req.Party.User)
			if err != nil {
				log.Debugf("[task=%s] err:%s ", taskID, err.Error())
				time.Sleep(time.Second)
				continue
			}
			if !joined {
				log.Debugf("[task=%s] wait for %s join", taskID, req.Party.User)
				time.Sleep(time.Second)
			} else {
				break
			}
		}
		log.Infof("[task=%s] ProjectMemberJoined ok", taskID)
	} else {
		// wait for create project
		for {
			joined, err := JoinProject()
			if err != nil {
				if strings.Contains(err.Error(), "record not found") {
					log.Debugf("[task=%s] err:%s ", taskID, err.Error())
					break
				}
				log.Debugf("[task=%s] err:%s ", taskID, err.Error())
				time.Sleep(time.Second)
				continue
			}
			if !joined {
				log.Debugf("[task=%s] wait for create project", taskID)
				time.Sleep(time.Second)
			} else {
				break
			}
		}
		log.Infof("[task=%s] JoinProject ok", taskID)
	}
	// create vtable
	for {
		err := createTable(req)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				log.Errorf("[task=%s] err:%s ", taskID, err.Error())
				break
			}
			log.Debugf("[task=%s] err:%s ", taskID, err.Error())
			time.Sleep(time.Second)
			continue
		} else {
			break
		}
	}
	log.Infof("[task=%s] createTable ok", taskID)

	for _, column := range req.Columns {
		err := grantCCL(req.User, req.User, column.Column, "PLAINTEXT")
		if err != nil {
			log.Error(err.Error())
			continue
		}
		for _, per := range column.Permissions {
			err = grantCCL(per.User, req.User, column.Column, per.Permission)
			if err != nil {
				log.Error(err.Error())
			}
		}
	}
	log.Infof("[task=%s] grantCCL ok", taskID)
	log.Infof("[task=%s] wait for party grant!!", taskID)

	if req.RunSQL == "" {
		log.Infof("[task=%s] RunSQL empty", taskID)
		return
	}
	log.Infof("[task=%s]  runQuery...", taskID)
	i = 0
	for {
		i++
		if i > 30 {
			log.Errorf("[task=%s]  too many attempts", taskID)
			break
		}
		err := runQuery(req.RunSQL, resultFile)
		if err != nil {
			log.Errorf("[task=%s] err:%s", taskID, err.Error())
			time.Sleep(time.Second)
			continue
		}
		if FileExists(resultFile) {
			log.Info(resultFile, " result success")
			data, err := os.ReadFile(resultFile)
			if err != nil {
				log.Errorf("[task=%s] err:%s", taskID, err.Error())
				break
			}
			resultFile := fmt.Sprintf("%s/tsql_result_%s.csv", filepath.Dir(req.Data), time.Now().Format("20060102150405"))
			result, err := client.WriteFile(context.Background(), resultFile, data)
			if err != nil {
				log.Errorf("[task=%s] err:%s", taskID, err.Error())
				break
			}
			log.Infof("[task=%s] nexusfs WriteFile etag:%s size %d", taskID, result.Etag, result.Size)
			log.Printf("[task=%s] privacy compute finished upload file to nexusfs: filepath: %s", taskID, resultFile)
			break
		}
	}
}
