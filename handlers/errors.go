package handlers

import (
	"github.com/gofiber/fiber/v2"
)

// ErrorCode represents a typed error code for client libraries
type ErrorCode string

const (
	// Validation errors (400)
	ErrorCodeMissingParameter      ErrorCode = "MISSING_PARAMETER"
	ErrorCodeInvalidParameter      ErrorCode = "INVALID_PARAMETER"
	ErrorCodeInvalidRequestBody    ErrorCode = "INVALID_REQUEST_BODY"
	ErrorCodeConflictingParameters ErrorCode = "CONFLICTING_PARAMETERS"
	ErrorCodeInvalidFormat         ErrorCode = "INVALID_FORMAT"
	ErrorCodeParseError            ErrorCode = "PARSE_ERROR"

	// Not found errors (404)
	ErrorCodeIndexNotFound    ErrorCode = "INDEX_NOT_FOUND"
	ErrorCodeDocumentNotFound ErrorCode = "DOCUMENT_NOT_FOUND"

	// Cluster errors (307/503)
	ErrorCodeNotLeader          ErrorCode = "NOT_LEADER"
	ErrorCodeClusterUnavailable ErrorCode = "CLUSTER_UNAVAILABLE"

	// Authorization errors (403)
	ErrorCodeInsufficientPermissions ErrorCode = "INSUFFICIENT_PERMISSIONS"
	ErrorCodeLeaderOnlyOperation     ErrorCode = "LEADER_ONLY_OPERATION"

	// Resource conflict errors (409)
	ErrorCodeResourceAlreadyExists ErrorCode = "RESOURCE_ALREADY_EXISTS"

	// Internal errors (500)
	ErrorCodeUUIDGenerationFailed   ErrorCode = "UUID_GENERATION_FAILED"
	ErrorCodeSerializationFailed    ErrorCode = "SERIALIZATION_FAILED"
	ErrorCodeRaftApplyFailed        ErrorCode = "RAFT_APPLY_FAILED"
	ErrorCodeIndexOperationFailed   ErrorCode = "INDEX_OPERATION_FAILED"
	ErrorCodeDocumentOperationFailed ErrorCode = "DOCUMENT_OPERATION_FAILED"
	ErrorCodeBatchOperationFailed   ErrorCode = "BATCH_OPERATION_FAILED"
	ErrorCodeSearchFailed           ErrorCode = "SEARCH_FAILED"
	ErrorCodeInternalError          ErrorCode = "INTERNAL_ERROR"
)

// ErrorResponse represents a structured error response
type ErrorResponse struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details string    `json:"details,omitempty"`
}

// ClusterErrorResponse extends ErrorResponse with cluster information
type ClusterErrorResponse struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Leader  string    `json:"leader,omitempty"`
}

// Error helper functions

func BadRequest(c *fiber.Ctx, code ErrorCode, message string) error {
	return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
		Code:    code,
		Message: message,
	})
}

func BadRequestWithDetails(c *fiber.Ctx, code ErrorCode, message, details string) error {
	return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
		Code:    code,
		Message: message,
		Details: details,
	})
}

func NotFound(c *fiber.Ctx, code ErrorCode, message string) error {
	return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
		Code:    code,
		Message: message,
	})
}

func Forbidden(c *fiber.Ctx, code ErrorCode, message string) error {
	return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
		Code:    code,
		Message: message,
	})
}

func ForbiddenWithLeader(c *fiber.Ctx, code ErrorCode, message, leader string) error {
	return c.Status(fiber.StatusForbidden).JSON(ClusterErrorResponse{
		Code:    code,
		Message: message,
		Leader:  leader,
	})
}

func Conflict(c *fiber.Ctx, code ErrorCode, message string) error {
	return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
		Code:    code,
		Message: message,
	})
}

func InternalError(c *fiber.Ctx, code ErrorCode, message string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
		Code:    code,
		Message: message,
	})
}

func InternalErrorWithDetails(c *fiber.Ctx, code ErrorCode, message, details string) error {
	return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
		Code:    code,
		Message: message,
		Details: details,
	})
}

func TemporaryRedirect(c *fiber.Ctx, leader string) error {
	return c.Status(fiber.StatusTemporaryRedirect).JSON(ClusterErrorResponse{
		Code:    ErrorCodeNotLeader,
		Message: "not leader",
		Leader:  leader,
	})
}
