package middleware

import (
	"bright/config"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// Authorization creates an authentication middleware
// If masterKey is empty, authentication is disabled and all requests are allowed
// Otherwise, validates Bearer token in Authorization header
func Authorization(cfg *config.Config, logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// If no master key is configured, skip authentication
		if !cfg.RequiresAuth() {
			return c.Next()
		}

		authHeader := c.Get("Authorization")
		if authHeader == "" {
			logger.Warn("missing authorization header",
				zap.String("path", c.Path()),
				zap.String("method", c.Method()),
				zap.String("ip", c.IP()),
			)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing authorization header",
			})
		}

		// Check for Bearer token format
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Warn("invalid authorization format",
				zap.String("path", c.Path()),
				zap.String("method", c.Method()),
				zap.String("ip", c.IP()),
			)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid authorization format, expected 'Bearer <token>'",
			})
		}

		token := parts[1]
		if token != cfg.MasterKey {
			logger.Warn("invalid authorization token",
				zap.String("path", c.Path()),
				zap.String("method", c.Method()),
				zap.String("ip", c.IP()),
			)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid authorization token",
			})
		}

		logger.Debug("request authorized",
			zap.String("path", c.Path()),
			zap.String("method", c.Method()),
		)

		return c.Next()
	}
}
