package handlers

import (
	"bright/ingresses"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
)

// IngressManager is the interface for the ingress manager
type IngressManager interface {
	Create(indexID string, ingressType string, id string, rawConfig json.RawMessage) (ingresses.Ingress, error)
	Get(id string) (ingresses.Ingress, error)
	List(indexID string) []ingresses.Ingress
	Delete(id string) error
}

// CreateIngressRequest is the request body for creating an ingress
type CreateIngressRequest struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Config json.RawMessage `json:"config"`
}

// ListIngresses returns all ingresses for an index
// GET /indexes/:id/ingresses
func ListIngresses(c *fiber.Ctx) error {
	ctx := GetContext(c)
	if ctx.IngressManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ingress manager not available",
		})
	}

	indexID := c.Params("id")

	// Verify index exists
	if _, _, err := ctx.Store.GetIndex(indexID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	ingressList := ctx.IngressManager.List(indexID)

	result := make([]ingresses.IngressInfo, 0, len(ingressList))
	for _, ing := range ingressList {
		result = append(result, ingresses.ToInfo(ing))
	}

	return c.JSON(fiber.Map{
		"ingresses": result,
	})
}

// CreateIngress creates a new ingress for an index
// POST /indexes/:id/ingresses
func CreateIngress(c *fiber.Ctx) error {
	ctx := GetContext(c)
	if ctx.IngressManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ingress manager not available",
		})
	}

	indexID := c.Params("id")

	// Verify index exists
	if _, _, err := ctx.Store.GetIndex(indexID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var req CreateIngressRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	if req.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "id is required",
		})
	}

	if req.Type == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "type is required",
		})
	}

	ing, err := ctx.IngressManager.Create(indexID, req.Type, req.ID, req.Config)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Auto-start the ingress
	if err := ing.Start(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "ingress created but failed to start",
			"details": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(ingresses.ToInfo(ing))
}

// GetIngress returns details of a specific ingress
// GET /indexes/:id/ingresses/:ingressId
func GetIngress(c *fiber.Ctx) error {
	ctx := GetContext(c)
	if ctx.IngressManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ingress manager not available",
		})
	}

	ingressID := c.Params("ingressId")

	ing, err := ctx.IngressManager.Get(ingressID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(ingresses.ToInfo(ing))
}

// DeleteIngress removes an ingress
// DELETE /indexes/:id/ingresses/:ingressId
func DeleteIngress(c *fiber.Ctx) error {
	ctx := GetContext(c)
	if ctx.IngressManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ingress manager not available",
		})
	}

	ingressID := c.Params("ingressId")

	if err := ctx.IngressManager.Delete(ingressID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// UpdateIngressRequest is the request body for updating an ingress
type UpdateIngressRequest struct {
	State string `json:"state"` // "resyncing", "paused", "running"
}

// UpdateIngress updates an ingress state
// PATCH /indexes/:id/ingresses/:ingressId
func UpdateIngress(c *fiber.Ctx) error {
	ctx := GetContext(c)
	if ctx.IngressManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "ingress manager not available",
		})
	}

	ingressID := c.Params("ingressId")

	ing, err := ctx.IngressManager.Get(ingressID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var req UpdateIngressRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	switch req.State {
	case "resyncing":
		if err := ing.Resync(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
	case "paused":
		if err := ing.Pause(); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
	case "running":
		if err := ing.Resume(); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid state, must be one of: resyncing, paused, running",
		})
	}

	return c.JSON(ingresses.ToInfo(ing))
}
