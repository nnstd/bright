package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

var startTime = time.Now()

// Health handles GET /health
func Health(c *fiber.Ctx) error {
	ctx := GetContext(c)

	health := fiber.Map{
		"status": "ok",
	}

	if IsRaftEnabled(c) {
		hasLeader := ctx.RaftNode.LeaderAddr() != ""
		health["raft"] = fiber.Map{
			"enabled":    true,
			"is_leader":  IsLeader(c),
			"has_leader": hasLeader,
		}

		// Allow a grace period for cluster formation (60 seconds)
		// During this time, return 200 even if there's no leader
		gracePeriod := 60 * time.Second
		timeSinceStart := time.Since(startTime)

		// Return 503 if cluster has no leader (unhealthy)
		// But only after the grace period has elapsed
		if !hasLeader && timeSinceStart > gracePeriod {
			health["status"] = "degraded"
			return c.Status(fiber.StatusServiceUnavailable).JSON(health)
		}
	}

	return c.JSON(health)
}
