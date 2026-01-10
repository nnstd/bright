package rpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
