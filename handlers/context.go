package handlers

import (
	"bright/config"
	"bright/raft"
	"bright/store"

	"github.com/gofiber/fiber/v2"
)

// HandlerContext holds dependencies needed by handlers
type HandlerContext struct {
	Store    *store.IndexStore
	RaftNode *raft.RaftNode
	Config   *config.Config
}

const contextKey = "handler_context"

// SetContext stores the HandlerContext in the Fiber context
func SetContext(c *fiber.Ctx, ctx *HandlerContext) {
	c.Locals(contextKey, ctx)
}

// GetContext retrieves the HandlerContext from the Fiber context
func GetContext(c *fiber.Ctx) *HandlerContext {
	return c.Locals(contextKey).(*HandlerContext)
}

// IsRaftEnabled returns true if Raft is enabled in this deployment
func IsRaftEnabled(c *fiber.Ctx) bool {
	ctx := GetContext(c)
	return ctx.RaftNode != nil
}

// IsLeader returns true if this node is the current Raft leader
func IsLeader(c *fiber.Ctx) bool {
	ctx := GetContext(c)
	return ctx.RaftNode != nil && ctx.RaftNode.IsLeader()
}
