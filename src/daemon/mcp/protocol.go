package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// JSONRPC 2.0 基础类型

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// 标准错误码
const (
	errParseError     = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternal       = -32603
)

// stdioTransport 通过 stdin/stdout 进行 JSON-RPC 通信
type stdioTransport struct {
	reader *bufio.Reader
	writer io.Writer
}

func newStdioTransport(r io.Reader, w io.Writer) *stdioTransport {
	return &stdioTransport{
		reader: bufio.NewReaderSize(r, 1<<20), // 1MB 缓冲
		writer: w,
	}
}

// readMessage 从 stdin 读取一行 JSON-RPC 消息
func (t *stdioTransport) readMessage() (*jsonrpcRequest, error) {
	line, err := t.reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, nil
	}
	var req jsonrpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return nil, err
	}
	if req.JSONRPC != "2.0" {
		return nil, fmt.Errorf("invalid jsonrpc version: %s", req.JSONRPC)
	}
	return &req, nil
}

// writeMessage 将响应写入 stdout
func (t *stdioTransport) writeMessage(resp *jsonrpcResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = t.writer.Write(data)
	return err
}

func makeResponse(id json.RawMessage, result interface{}) *jsonrpcResponse {
	return &jsonrpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func makeError(id json.RawMessage, code int, msg string) *jsonrpcResponse {
	return &jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &jsonrpcError{Code: code, Message: msg},
	}
}
