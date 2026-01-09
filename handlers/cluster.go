package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// ClusterStatus returns the current cluster status
func ClusterStatus(c *fiber.Ctx) error {
	ctx := GetContext(c)

	if !IsRaftEnabled(c) {
		return c.JSON(fiber.Map{
			"mode":    "standalone",
			"healthy": true,
		})
	}

	return c.JSON(fiber.Map{
		"mode":      "clustered",
		"node_id":   ctx.RaftNode.GetConfig().NodeID,
		"is_leader": IsLeader(c),
		"leader":    ctx.RaftNode.LeaderAddr(),
	})
}

// JoinCluster adds a new node to the Raft cluster
func JoinCluster(c *fiber.Ctx) error {
	var req struct {
		NodeID string `json:"node_id"`
		Addr   string `json:"addr"`
	}

	if err := c.BodyParser(&req); err != nil {
		return BadRequest(c, ErrorCodeInvalidRequestBody, "invalid request body")
	}

	if req.NodeID == "" || req.Addr == "" {
		return BadRequest(c, ErrorCodeMissingParameter, "node_id and addr are required")
	}

	ctx := GetContext(c)

	if !IsLeader(c) {
		return ForbiddenWithLeader(c, ErrorCodeLeaderOnlyOperation, "only leader can add nodes", ctx.RaftNode.LeaderAddr())
	}

	if err := ctx.RaftNode.Join(req.NodeID, req.Addr); err != nil {
		return InternalErrorWithDetails(c, ErrorCodeClusterUnavailable, "failed to join node to cluster", err.Error())
	}

	return c.JSON(fiber.Map{
		"status":  "joined",
		"node_id": req.NodeID,
	})
}
