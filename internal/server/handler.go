// Package server exposes the HTTP runtime and middleware stack.
package server

import (
	"errors"
	"github.com/samber/oops"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v3"
)

func newErrorHandler(logger *slog.Logger) fiber.ErrorHandler {
	return func(ctx fiber.Ctx, err error) error {
		return errorHandler(ctx, logger, err)
	}
}

func errorHandler(ctx fiber.Ctx, logger *slog.Logger, err error) error {
	code := fiber.StatusInternalServerError

	if fiberErr, ok := errors.AsType[*fiber.Error](err); ok {
		code = fiberErr.Code
	}

	switch code {
	case fiber.StatusNotFound:
		return sendErrorResponse(ctx, fiber.StatusNotFound, "Not found")
	default:
		requestID := responseRequestID(ctx)
		logRequestError(ctx, logger, err, requestID, code)
		return sendErrorResponse(ctx, fiber.StatusInternalServerError, internalErrorResponseBody(requestID))
	}
}

func responseRequestID(ctx fiber.Ctx) string {
	requestID := strings.TrimSpace(ctx.GetRespHeader(RequestIDHeader))
	if requestID == "" {
		requestID = strings.TrimSpace(ctx.Get(RequestIDHeader))
	}
	if requestID == "" {
		return "unknown"
	}
	return requestID
}

func internalErrorResponseBody(requestID string) string {
	return "Internal Server Error\nrequest_id=" + requestID
}

func logRequestError(ctx fiber.Ctx, logger *slog.Logger, err error, requestID string, code int) {
	if logger == nil {
		return
	}
	logger.Error("HTTP request failed",
		slog.String("request_id", requestID),
		slog.Int("status", code),
		slog.String("method", ctx.Method()),
		slog.String("path", ctx.Path()),
		slog.String("err", err.Error()),
	)
}

func sendErrorResponse(ctx fiber.Ctx, code int, body string) error {
	ctx.Set(fiber.HeaderContentType, fiber.MIMETextPlainCharsetUTF8)
	if err := ctx.Status(code).SendString(body); err != nil {
		return oops.Wrapf(err, "send %d response body", code)
	}
	return nil
}
