package rpc

import "context"

// RPCClient defines the interface for internal RPC operations
type RPCClient interface {
	// ForwardRequest forwards an HTTP request to the leader node
	ForwardRequest(ctx context.Context, leaderAddr string, req *ForwardedRequest) (*ForwardedResponse, error)
}

// ForwardedRequest represents an HTTP request to be forwarded to the leader
type ForwardedRequest struct {
	Method      string            // HTTP method (POST, DELETE, PATCH, etc.)
	Path        string            // Request path with parameters
	Body        []byte            // Request body
	Headers     map[string]string // HTTP headers to preserve
	QueryParams map[string]string // Query parameters
}

// ForwardedResponse represents the response from a forwarded request
type ForwardedResponse struct {
	StatusCode int               // HTTP status code
	Body       []byte            // Response body
	Headers    map[string]string // Response headers
}
