package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"gitlab.ozon.dev/gojhw1/pkg/metrics"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

type logger interface {
	Log(ctx context.Context, log model.AuditLog)
}

// AuditMiddleware создает middleware для логирования запросов и ответов
func AuditMiddleware(logger logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.Context()

		requestID := c.Get(fiber.HeaderXRequestID)
		if requestID == "" {
			requestID = fmt.Sprintf("%d", time.Now().UnixNano())
			c.Set(fiber.HeaderXRequestID, requestID)
		}

		requestBody := c.Body()
		var reqBody any
		if len(requestBody) > 0 && len(requestBody) < 1024 { // 1024 - ограничение, чтобы не логировать слишком большие тела запросов
			if err := json.Unmarshal(requestBody, &reqBody); err != nil {
				reqBody = string(requestBody)
			}
		}

		logger.Log(ctx, model.AuditLog{
			Timestamp: time.Now(),
			Type:      model.AuditLogTypeRequest,
			Path:      c.Path(),
			Method:    c.Method(),
			RequestID: requestID,
			IP:        c.IP(),
			Body:      reqBody,
		})

		err := c.Next()

		var respBody any
		responseBody := c.Response().Body()
		if len(responseBody) > 0 && len(responseBody) < 1024 {
			if err := json.Unmarshal(responseBody, &respBody); err != nil {
				respBody = string(responseBody)
			}
		}

		logger.Log(ctx, model.AuditLog{
			Timestamp:  time.Now(),
			StatusCode: c.Response().StatusCode(),
			Type:       model.AuditLogTypeResponse,
			Path:       c.Path(),
			Method:     c.Method(),
			RequestID:  requestID,
			IP:         c.IP(),
			Body:       respBody,
		})

		return err
	}
}

func MetricsMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		var statusCode int

		defer func() {
			duration := time.Since(start).Seconds()

			if statusCode == 0 {
				statusCode = c.Response().StatusCode()
			}

			metrics.HttpRequestsTotal.WithLabelValues(c.Method(), c.Path(), strconv.Itoa(statusCode)).Inc()
			metrics.HttpRequestDuration.WithLabelValues(c.Method(), c.Path()).Observe(duration)
		}()

		err := c.Next()

		if err != nil {
			if fiberErr, ok := err.(*fiber.Error); ok {
				statusCode = fiberErr.Code
			} else {
				statusCode = fiber.StatusInternalServerError
			}
		}

		return err
	}
}
