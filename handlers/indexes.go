package handlers

import (
	"bright/models"
	"bright/raft"
	"bright/store"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
)

// ListIndexes handles GET /indexes
func ListIndexes(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 20)
	offset := c.QueryInt("offset", 0)
	page := c.QueryInt("page", 0)

	// If page is provided, calculate offset from page
	if page > 0 {
		offset = (page - 1) * limit
	}

	s := store.GetStore()
	items := s.ListIndexes(limit, offset)

	return c.JSON(fiber.Map{
		"items": items,
	})
}

// CreateIndex handles POST /indexes
func CreateIndex(c *fiber.Ctx) error {
	id := c.Query("id")
	primaryKey := c.Query("primaryKey")

	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "id parameter is required",
		})
	}

	// Make copies of the strings to avoid Fiber buffer reuse issues
	id = utils.CopyString(id)
	primaryKey = utils.CopyString(primaryKey)

	ctx := GetContext(c)

	// If Raft is enabled, apply command through consensus
	if IsRaftEnabled(c) {
		if !IsLeader(c) {
			// Redirect to leader
			return c.Status(fiber.StatusTemporaryRedirect).JSON(fiber.Map{
				"error":  "not leader",
				"leader": ctx.RaftNode.LeaderAddr(),
			})
		}

		// Apply command via Raft
		cmd := raft.Command{
			Type: raft.CommandCreateIndex,
			Data: json.RawMessage(fmt.Sprintf(`{"id":"%s","primaryKey":"%s"}`, id, primaryKey)),
		}

		if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		config := &models.IndexConfig{
			ID:         id,
			PrimaryKey: primaryKey,
		}
		return c.Status(fiber.StatusCreated).JSON(config)
	}

	// Single-node mode: apply directly
	config := &models.IndexConfig{
		ID:         id,
		PrimaryKey: primaryKey,
	}

	s := store.GetStore()
	if err := s.CreateIndex(config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(config)
}

// DeleteIndex handles DELETE /indexes/:id
func DeleteIndex(c *fiber.Ctx) error {
	id := c.Params("id")

	ctx := GetContext(c)

	// If Raft is enabled, apply command through consensus
	if IsRaftEnabled(c) {
		if !IsLeader(c) {
			// Redirect to leader
			return c.Status(fiber.StatusTemporaryRedirect).JSON(fiber.Map{
				"error":  "not leader",
				"leader": ctx.RaftNode.LeaderAddr(),
			})
		}

		// Apply command via Raft
		cmd := raft.Command{
			Type: raft.CommandDeleteIndex,
			Data: json.RawMessage(fmt.Sprintf(`{"id":"%s"}`, id)),
		}

		if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		return c.Status(fiber.StatusNoContent).Send(nil)
	}

	// Single-node mode: apply directly
	s := store.GetStore()
	if err := s.DeleteIndex(id); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusNoContent).Send(nil)
}

// UpdateIndex handles PATCH /indexes/:id
func UpdateIndex(c *fiber.Ctx) error {
	id := c.Params("id")

	var config models.IndexConfig
	if err := c.BodyParser(&config); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	ctx := GetContext(c)

	// If Raft is enabled, apply command through consensus
	if IsRaftEnabled(c) {
		if !IsLeader(c) {
			// Redirect to leader
			return c.Status(fiber.StatusTemporaryRedirect).JSON(fiber.Map{
				"error":  "not leader",
				"leader": ctx.RaftNode.LeaderAddr(),
			})
		}

		// Apply command via Raft
		cmd := raft.Command{
			Type: raft.CommandUpdateIndex,
			Data: json.RawMessage(fmt.Sprintf(`{"id":"%s","primaryKey":"%s"}`, id, config.PrimaryKey)),
		}

		if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		config.ID = id
		return c.JSON(config)
	}

	// Single-node mode: apply directly
	s := store.GetStore()
	if err := s.UpdateIndex(id, &config); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(config)
}
