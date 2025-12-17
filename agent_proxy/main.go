package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

func init() {
	file, err := os.OpenFile("agent_proxy.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
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

// Config / persistence file
const (
	mappingsFile       = "config/mappings.json"
	configFile         = "config/config.json"
	proxyErrorHeader   = "X-Proxy-Error"
	defaultContentType = "application/json"
)

type Config struct {
	PrivacyAgentURL string `json:"privacy_agent_url"`
	NexusServerURL  string `json:"nexus_server_url"`
}

// Mappings persisted to disk
type Mappings struct {
	mu sync.RWMutex `json:"-"`

	UserToNexusKey map[string]string `json:"user_to_agent_key"`
}

func NewMappings() *Mappings {
	return &Mappings{
		UserToNexusKey: make(map[string]string),
	}
}

func (m *Mappings) Load(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no file yet
		}
		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	tmp := &Mappings{}
	if err := dec.Decode(tmp); err != nil {
		return err
	}

	m.UserToNexusKey = tmp.UserToNexusKey
	return nil
}

func (m *Mappings) Save(path string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tmp := &Mappings{
		UserToNexusKey: m.UserToNexusKey,
	}

	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	tmpFile := path + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(tmp); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmpFile, path)
}

func (m *Mappings) Register(userID, agentKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if userID != "" {
		m.UserToNexusKey[userID] = agentKey
	}
}

func (m *Mappings) GetNexusKeyByUser(userID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	nexusKey, ok := m.UserToNexusKey[userID]
	if !ok {
		return "", errors.New("no agent key for user")
	}
	return nexusKey, nil
}

// Global state
var (
	mapping   *Mappings
	cfg       Config
	targetURL *url.URL
)

func main() {
	// Initialize global state
	mapping = NewMappings()

	// Load mappings
	if err := mapping.Load(mappingsFile); err != nil {
		log.Fatalf("failed to load mappings: %v", err)
	}
	log.Printf("loaded mappings from %s", mappingsFile)

	// Load config
	if err := loadConfig(configFile, &cfg); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Parse target URL once at startup
	var err error
	targetURL, err = url.Parse(cfg.PrivacyAgentURL)
	if err != nil {
		log.Fatalf("failed to parse privacy agent URL: %v", err)
	}
	log.Printf("proxy target: %s", targetURL.String())

	// Setup routes
	http.HandleFunc("/register", registerHandler(mappingsFile))
	http.HandleFunc("/", GenericProxyHandler)

	srv := &http.Server{
		Addr:         ":2024",
		ReadTimeout:  0,
		WriteTimeout: 0,
		IdleTimeout:  0,
	}
	log.Printf("listening on :2024")
	log.Fatal(srv.ListenAndServe())
}

// loadConfig loads configuration from JSON file
func loadConfig(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return nil
}

// registerHandler handles user registration requests
func registerHandler(mappingsPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			UserID   string `json:"user_id"`
			NexusKey string `json:"nexus_key"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.UserID == "" || req.NexusKey == "" {
			http.Error(w, "user_id and nexus_key are required", http.StatusBadRequest)
			return
		}

		mapping.Register(req.UserID, req.NexusKey)
		if err := mapping.Save(mappingsPath); err != nil {
			log.Printf("warning: failed to save mappings: %v", err)
		}

		w.Header().Set("Content-Type", defaultContentType)
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

// GenericProxyHandler forwards any path to the user's agent using ReverseProxy
func GenericProxyHandler(w http.ResponseWriter, r *http.Request) {
	// Read request body before creating proxy
	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body.Close()

	// Create reverse proxy
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			if req.Method != http.MethodPost {
				setupTargetRequest(req, targetURL, bodyBytes)
				return
			}
			modifiedBody := bodyBytes
			log.Printf("bodyBytes:%s", string(bodyBytes))
			md, err := extractMetadata(bodyBytes)
			if err != nil {
				log.Printf("failed to extract metadata %s %s ", req.Method, r.URL.Path)
				req.Header.Set(proxyErrorHeader, err.Error())
			} else {
				log.Printf("extractMetadata:UserID[%s] TargetUserID[%s] XAuth[%s]", md.UserID, md.TargetUserID, md.XAuth)
				// Auto-register if not exists
				if md.UserID != "" {
					if _, err := mapping.GetNexusKeyByUser(md.UserID); err != nil {
						mapping.Register(md.UserID, md.XAuth)
						if err := mapping.Save(mappingsFile); err != nil {
							log.Printf("warning: failed to save mappings: %v", err)
						}
						log.Printf("registered user %s XAuth %s", md.UserID, md.XAuth)
					}
				}

				// Determine target user and auth
				if md.TargetUserID != "" {
					xAuth, err := mapping.GetNexusKeyByUser(md.TargetUserID)
					if err != nil {
						log.Printf("no agent mapping for user %s: %v", md.TargetUserID, err)
						req.Header.Set(proxyErrorHeader, "no agent mapping: "+err.Error())
					} else {
						// Modify request body: replace metadata with target user's info
						modifiedBody = modifyMetadata(bodyBytes, xAuth, md.TargetUserID)
					}
				}
			}
			setupTargetRequest(req, targetURL, modifiedBody)
			log.Printf("proxying %s %s -> %s", req.Method, r.URL.Path, req.URL.String())
		},
		ModifyResponse: func(resp *http.Response) error {
			// Check for errors set in Director
			if errMsg := resp.Request.Header.Get(proxyErrorHeader); errMsg != "" {
				resp.StatusCode = http.StatusBadRequest
				resp.Status = http.StatusText(http.StatusBadRequest)
				resp.Body = io.NopCloser(strings.NewReader(errMsg))
				resp.ContentLength = int64(len(errMsg))
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("proxy error: %v", err)
			http.Error(w, "proxy request failed: "+err.Error(), http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)
}

// setupTargetRequest configures the request to be sent to the target
func setupTargetRequest(req *http.Request, targetURL *url.URL, body []byte) {
	req.URL.Scheme = targetURL.Scheme
	req.URL.Host = targetURL.Host
	req.URL.Path = strings.TrimRight(targetURL.Path, "/") + req.URL.Path
	req.Host = targetURL.Host
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
}

// modifyMetadata modifies the request body to replace metadata with target user info
func modifyMetadata(bodyBytes []byte, xAuth, userID string) []byte {
	if len(bodyBytes) == 0 || xAuth == "" {
		return bodyBytes
	}

	var tmp map[string]any
	if err := json.Unmarshal(bodyBytes, &tmp); err != nil {
		return bodyBytes
	}

	mdMap, ok := tmp["metadata"].(map[string]any)
	if !ok {
		return bodyBytes
	}

	mdMap["x_auth"] = xAuth
	mdMap["user_id"] = userID
	mdMap["nexus_server_url"] = cfg.NexusServerURL
	tmp["metadata"] = mdMap

	modifiedBody, err := json.Marshal(tmp)
	if err != nil {
		return bodyBytes
	}

	return modifiedBody
}

type Metadata struct {
	UserID       string `json:"user_id"`
	XAuth        string `json:"x_auth"`
	TargetUserID string `json:"target_user_id"`
}

type requestData struct {
	Metadata Metadata `json:"metadata"`
}

// extractMetadata extracts metadata from request body
func extractMetadata(body []byte) (*Metadata, error) {
	if len(body) == 0 {
		return nil, errors.New("body empty")
	}
	var tmp requestData
	if err := json.Unmarshal(body, &tmp); err != nil {
		return nil, err
	}
	return &tmp.Metadata, nil
}
