package rpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

// HTTPRPCClient implements the RPCClient interface using HTTP
type HTTPRPCClient struct {
	httpClient *http.Client
	timeout    time.Duration
	logger     *zap.Logger
}

// NewHTTPRPCClient creates a new HTTP-based RPC client
func NewHTTPRPCClient(logger *zap.Logger) *HTTPRPCClient {
	return &HTTPRPCClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		timeout: 10 * time.Second,
		logger:  logger,
	}
}

// ForwardRequest forwards an HTTP request to the leader node
func (c *HTTPRPCClient) ForwardRequest(ctx context.Context, leaderRaftAddr string, req *ForwardedRequest) (*ForwardedResponse, error) {
	// Convert Raft address (port 7000) to HTTP address (port 3000)
	httpAddr := convertRaftAddrToHTTP(leaderRaftAddr)

	// Construct full URL
	url := fmt.Sprintf("http://%s%s", httpAddr, req.Path)

	// Add query parameters if present
	if len(req.QueryParams) > 0 {
		url += "?"
		first := true
		for key, value := range req.QueryParams {
			if !first {
				url += "&"
			}
			url += fmt.Sprintf("%s=%s", key, value)
			first = false
		}
	}

	c.logger.Info("Forwarding request to leader",
		zap.String("method", req.Method),
		zap.String("path", req.Path),
		zap.String("leader", httpAddr),
	)

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, bytes.NewReader(req.Body))
	if err != nil {
		c.logger.Error("Failed to create forwarding request", zap.Error(err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Copy headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Execute request
	startTime := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.Error("Failed to forward request",
			zap.Error(err),
			zap.String("leader", httpAddr),
		)
		return nil, fmt.Errorf("failed to forward request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.Error("Failed to read forwarded response", zap.Error(err))
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	duration := time.Since(startTime)
	c.logger.Info("Request forwarding completed",
		zap.Int("status", resp.StatusCode),
		zap.Duration("latency", duration),
	)

	// Extract response headers
	respHeaders := make(map[string]string)
	for key := range resp.Header {
		respHeaders[key] = resp.Header.Get(key)
	}

	return &ForwardedResponse{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    respHeaders,
	}, nil
}

// convertRaftAddrToHTTP converts a Raft address (port 7000) to HTTP API address (port 3000)
func convertRaftAddrToHTTP(raftAddr string) string {
	return strings.Replace(raftAddr, ":7000", ":3000", 1)
}

// ClusterJoin sends a cluster join request to a peer node
func (c *HTTPRPCClient) ClusterJoin(ctx context.Context, peerRaftAddr, nodeID, addr, masterKey string) error {
	// Convert Raft address (port 7000) to HTTP address (port 3000)
	httpAddr := convertRaftAddrToHTTP(peerRaftAddr)

	// Prepare join request
	joinReq := map[string]string{
		"node_id": nodeID,
		"addr":    addr,
	}

	jsonData, err := sonic.Marshal(joinReq)
	if err != nil {
		c.logger.Error("Failed to marshal join request", zap.Error(err))
		return fmt.Errorf("failed to marshal join request: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("http://%s/cluster/join", httpAddr)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		c.logger.Error("Failed to create join request", zap.Error(err))
		return fmt.Errorf("failed to create join request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if masterKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", masterKey))
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Warn("Failed to contact peer",
			zap.String("peer", httpAddr),
			zap.Error(err),
		)
		return fmt.Errorf("failed to contact peer: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		c.logger.Warn("Join request failed",
			zap.String("peer", httpAddr),
			zap.Int("status", resp.StatusCode),
			zap.String("response", string(body)),
		)
		return fmt.Errorf("join request failed with status %d: %s", resp.StatusCode, string(body))
	}

	c.logger.Info("Successfully joined cluster",
		zap.String("peer", httpAddr),
		zap.String("node_id", nodeID),
	)

	return nil
}
