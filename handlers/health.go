package handlers

import "github.com/gofiber/fiber/v2"

// Health handles GET /health
func Health(c *fiber.Ctx) error {
	ctx := GetContext(c)

	health := fiber.Map{
		"status": "ok",
	}

	if IsRaftEnabled(c) {
		health["raft"] = fiber.Map{
			"enabled":    true,
			"is_leader":  IsLeader(c),
			"has_leader": ctx.RaftNode.LeaderAddr() != "",
		}

		// Return 503 if cluster has no leader (unhealthy)
		if ctx.RaftNode.LeaderAddr() == "" {
			health["status"] = "degraded"
			return c.Status(fiber.StatusServiceUnavailable).JSON(health)
		}
	}

	return c.JSON(health)
}
