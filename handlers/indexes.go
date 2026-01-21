package handlers

import (
	"bright/errors"
	"bright/models"
	"bright/raft"
	"bright/rpc"
	"bright/store"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
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
		return errors.BadRequest(c, errors.ErrorCodeMissingParameter, "id parameter is required")
	}

	// Make copies of the strings to avoid Fiber buffer reuse issues
	id = utils.CopyString(id)
	primaryKey = utils.CopyString(primaryKey)

	// Parse request body for additional options
	var reqBody struct {
		ExcludeAttributes []string `json:"excludeAttributes"`
	}
	c.BodyParser(&reqBody)

	ctx := GetContext(c)

	// If Raft is enabled, apply command through consensus
	if IsRaftEnabled(c) {
		if !IsLeader(c) {
			// Forward to leader
			return rpc.ForwardToLeader(c, ctx.RPCClient, ctx.RaftNode.LeaderAddr())
		}

		// Build config JSON with exclude attributes
		config := &models.IndexConfig{
			ID:                id,
			PrimaryKey:        primaryKey,
			ExcludeAttributes: reqBody.ExcludeAttributes,
		}
		configJSON, _ := sonic.Marshal(config)

		// Apply command via Raft
		cmd := raft.Command{
			Type: raft.CommandCreateIndex,
			Data: json.RawMessage(configJSON),
		}

		if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
			return errors.InternalErrorWithDetails(c, errors.ErrorCodeRaftApplyFailed, "failed to create index via Raft", err.Error())
		}

		return c.Status(fiber.StatusCreated).JSON(config)
	}

	// Single-node mode: apply directly
	config := &models.IndexConfig{
		ID:                id,
		PrimaryKey:        primaryKey,
		ExcludeAttributes: reqBody.ExcludeAttributes,
	}

	s := store.GetStore()
	if err := s.CreateIndex(config); err != nil {
		// Check if it's a duplicate index error
		if err.Error() == fmt.Sprintf("index %s already exists", id) {
			return errors.Conflict(c, errors.ErrorCodeResourceAlreadyExists, err.Error())
		}
		return errors.InternalErrorWithDetails(c, errors.ErrorCodeIndexOperationFailed, "failed to create index", err.Error())
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
			// Forward to leader
			return rpc.ForwardToLeader(c, ctx.RPCClient, ctx.RaftNode.LeaderAddr())
		}

		// Apply command via Raft
		cmd := raft.Command{
			Type: raft.CommandDeleteIndex,
			Data: json.RawMessage(fmt.Sprintf(`{"id":"%s"}`, id)),
		}

		if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
			return errors.InternalErrorWithDetails(c, errors.ErrorCodeRaftApplyFailed, "failed to delete index via Raft", err.Error())
		}

		return c.Status(fiber.StatusNoContent).Send(nil)
	}

	// Single-node mode: apply directly
	s := store.GetStore()
	if err := s.DeleteIndex(id); err != nil {
		return errors.NotFound(c, errors.ErrorCodeIndexNotFound, err.Error())
	}

	return c.Status(fiber.StatusNoContent).Send(nil)
}

// UpdateIndex handles PATCH /indexes/:id
func UpdateIndex(c *fiber.Ctx) error {
	id := c.Params("id")

	var config models.IndexConfig
	if err := c.BodyParser(&config); err != nil {
		return errors.BadRequest(c, errors.ErrorCodeInvalidRequestBody, "invalid request body")
	}

	ctx := GetContext(c)

	// If Raft is enabled, apply command through consensus
	if IsRaftEnabled(c) {
		if !IsLeader(c) {
			// Forward to leader
			return rpc.ForwardToLeader(c, ctx.RPCClient, ctx.RaftNode.LeaderAddr())
		}

		// Ensure ID is set and serialize full config
		config.ID = id
		configJSON, _ := sonic.Marshal(config)

		// Apply command via Raft
		cmd := raft.Command{
			Type: raft.CommandUpdateIndex,
			Data: json.RawMessage(configJSON),
		}

		if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
			return errors.InternalErrorWithDetails(c, errors.ErrorCodeRaftApplyFailed, "failed to update index via Raft", err.Error())
		}

		return c.JSON(config)
	}

	// Single-node mode: apply directly
	s := store.GetStore()
	if err := s.UpdateIndex(id, &config); err != nil {
		return errors.NotFound(c, errors.ErrorCodeIndexNotFound, err.Error())
	}

	return c.JSON(config)
}
