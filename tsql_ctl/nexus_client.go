package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ReadRequest struct {
	JSONRPC string     `json:"jsonrpc"`
	Method  string     `json:"method"`
	Params  ReadParams `json:"params"`
	ID      int64      `json:"id"`
}

type ReadParams struct {
	Path string `json:"path"`
}

type ReadResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Result  *ReadResult `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type ReadResult struct {
	Type string `json:"__type__"`
	Data string `json:"data"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type WriteRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  WriteParams `json:"params"`
	ID      int64       `json:"id"`
}

type WriteParams struct {
	Path    string       `json:"path"`
	Content BytesContent `json:"content"`
}

type BytesContent struct {
	Type string `json:"__type__"`
	Data string `json:"data"`
}

type WriteResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      int64        `json:"id"`
	Result  *WriteResult `json:"result,omitempty"`
	Error   *RPCError    `json:"error,omitempty"`
}

type WriteResult struct {
	Etag string `json:"etag"`
	Size int64  `json:"size"`
}

type Client struct {
	BaseURL string
	Auth    string
	Client  *http.Client
}

func NewClient(baseURL, auth string) *Client {
	return &Client{
		BaseURL: baseURL,
		Auth:    auth,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) ReadFile(ctx context.Context, path string) ([]byte, error) {
	reqBody := ReadRequest{
		JSONRPC: "2.0",
		Method:  "read",
		Params: ReadParams{
			Path: path,
		},
		ID: time.Now().UnixNano(),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/api/nfs/read", //http://124.223.11.17:8080/api/nfs/read
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Auth)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp ReadResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s",
			rpcResp.Error.Code,
			rpcResp.Error.Message,
		)
	}

	if rpcResp.Result == nil {
		return nil, fmt.Errorf("empty result")
	}

	// Base64 解码
	data, err := base64.StdEncoding.DecodeString(rpcResp.Result.Data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (c *Client) WriteFile(
	ctx context.Context,
	path string,
	data []byte,
) (*WriteResult, error) {

	// Base64 编码
	b64 := base64.StdEncoding.EncodeToString(data)

	reqBody := WriteRequest{
		JSONRPC: "2.0",
		Method:  "write",
		Params: WriteParams{
			Path: path,
			Content: BytesContent{
				Type: "bytes",
				Data: b64,
			},
		},
		ID: time.Now().UnixNano(),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.BaseURL+"/api/nfs/write",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Auth)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var rpcResp WriteResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, err
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf(
			"rpc error %d: %s",
			rpcResp.Error.Code,
			rpcResp.Error.Message,
		)
	}

	if rpcResp.Result == nil {
		return nil, fmt.Errorf("empty write result")
	}

	return rpcResp.Result, nil
}
