package rpc

import (
	"context"
	"fmt"

	brerrors "bright/errors"

	"github.com/gofiber/fiber/v2"
)

// ForwardToLeader forwards the current request to the leader node
func ForwardToLeader(c *fiber.Ctx, rpcClient RPCClient, leaderRaftAddr string) error {
	if rpcClient == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "RPC client not initialized",
		})
	}

	// Extract request details from Fiber context
	req := &ForwardedRequest{
		Method:      c.Method(),
		Path:        c.Path(),
		Body:        c.Body(),
		Headers:     make(map[string]string),
		QueryParams: make(map[string]string),
	}

	// Preserve critical headers
	if auth := c.Get("Authorization"); auth != "" {
		req.Headers["Authorization"] = auth
	}
	if contentType := c.Get("Content-Type"); contentType != "" {
		req.Headers["Content-Type"] = contentType
	}

	// Extract query parameters
	for key, value := range c.Request().URI().QueryArgs().All() {
		req.QueryParams[string(key)] = string(value)
	}

	// Forward request to leader
	ctx, cancel := context.WithTimeout(context.Background(), rpcClient.(*HTTPRPCClient).timeout)
	defer cancel()

	resp, err := rpcClient.ForwardRequest(ctx, leaderRaftAddr, req)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"code":    brerrors.ErrorCodeClusterUnavailable,
			"message": fmt.Sprintf("Failed to forward request to leader: %v", err),
			"leader":  leaderRaftAddr,
		})
	}

	// Set response headers
	for key, value := range resp.Headers {
		// Skip headers that Fiber handles automatically
		if key != "Content-Length" && key != "Transfer-Encoding" {
			c.Set(key, value)
		}
	}

	// Return leader's response
	return c.Status(resp.StatusCode).Send(resp.Body)
}
