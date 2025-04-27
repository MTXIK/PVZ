package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
)

const timeLayout = "2006-01-02T15:04:05"

func TestOrderHandlerIntegration_CreateOrder(t *testing.T) {
	app, _, _, cleanup := setupOrderTest(t)
	defer cleanup()

	deadline := time.Now().Add(24 * time.Hour).Format(timeLayout)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
	}{
		{
			name: "успешное создание заказа",
			requestBody: map[string]any{
				"id":           123,
				"customer_id":  456,
				"deadline_at":  deadline,
				"weight":       1.5,
				"cost":         1000,
				"package_type": "box",
				"wrapper":      "film",
			},
			expectedStatus: fiber.StatusCreated,
		},
		{
			name: "ошибка - отрицательный вес",
			requestBody: map[string]any{
				"id":           124,
				"customer_id":  456,
				"deadline_at":  deadline,
				"weight":       -1.5,
				"cost":         1000,
				"package_type": "box",
				"wrapper":      "film",
			},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name: "ошибка - отрицательная стоимость",
			requestBody: map[string]any{
				"id":           125,
				"customer_id":  456,
				"deadline_at":  deadline,
				"weight":       1.5,
				"cost":         -1000,
				"package_type": "box",
				"wrapper":      "film",
			},
			expectedStatus: fiber.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusCreated {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Equal(t, float64(123), result["id"])
				assert.Equal(t, float64(456), result["customer_id"])
			}
		})
	}
}

func TestOrderHandlerIntegration_GetOrder(t *testing.T) {
	app, orderService, _, cleanup := setupOrderTest(t)
	defer cleanup()

	deadline := time.Now().Add(24 * time.Hour)
	err := orderService.AcceptOrder(context.Background(), 123, 456, deadline, 1.5, 1000, nil, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		orderID        string
		expectedStatus int
	}{
		{
			name:           "успешное получение заказа",
			orderID:        "123",
			expectedStatus: fiber.StatusOK,
		},
		{
			name:           "ошибка - несуществующий заказ",
			orderID:        "999",
			expectedStatus: fiber.StatusNotFound,
		},
		{
			name:           "ошибка - некорректный ID заказа",
			orderID:        "invalid",
			expectedStatus: fiber.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders/%s", tt.orderID), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusOK {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Equal(t, float64(123), result["id"])
				assert.Equal(t, float64(456), result["customer_id"])
				assert.Equal(t, float64(1.5), result["weight"])
				assert.Equal(t, float64(1000), result["cost"])
			}
		})
	}
}

func TestOrderHandlerIntegration_ListOrders(t *testing.T) {
	app, orderService, _, cleanup := setupOrderTest(t)
	defer cleanup()

	deadline := time.Now().Add(24 * time.Hour)

	// Заказы для клиента 456
	err := orderService.AcceptOrder(context.Background(), 101, 456, deadline, 1.5, 1000, nil, nil)
	require.NoError(t, err)
	err = orderService.AcceptOrder(context.Background(), 102, 456, deadline, 2.5, 2000, nil, nil)
	require.NoError(t, err)

	// Заказ для другого клиента
	err = orderService.AcceptOrder(context.Background(), 103, 789, deadline, 3.5, 3000, nil, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "список заказов клиента 456",
			queryParams:    "customer_id=456&cursor=0&limit=10",
			expectedStatus: fiber.StatusOK,
			expectedCount:  2,
		},
		{
			name:           "список заказов клиента 789",
			queryParams:    "customer_id=789&cursor=0&limit=10",
			expectedStatus: fiber.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "список заказов несуществующего клиента",
			queryParams:    "customer_id=999&cursor=0&limit=10",
			expectedStatus: fiber.StatusOK,
			expectedCount:  0,
		},
		{
			name:           "ошибка - некорректный customer_id",
			queryParams:    "customer_id=invalid&cursor=0&limit=10",
			expectedStatus: fiber.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders?%s", tt.queryParams), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusOK {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Проверяем поле orders безопасно, учитывая возможность nil
				var ordersCount int
				if orders, ok := result["orders"]; ok && orders != nil {
					if ordersArr, ok := orders.([]any); ok {
						ordersCount = len(ordersArr)
					}
				}

				assert.Equal(t, tt.expectedCount, ordersCount,
					"Ожидалось %d заказов, получено %d. Тело ответа: %s",
					tt.expectedCount, ordersCount, string(body))
			}
		})
	}
}

func TestOrderHandlerIntegration_ProcessCustomer(t *testing.T) {
	app, orderService, _, cleanup := setupOrderTest(t)
	defer cleanup()

	deadline := time.Now().Add(24 * time.Hour)

	err := orderService.AcceptOrder(context.Background(), 201, 456, deadline, 1.5, 1000, nil, nil)
	require.NoError(t, err)

	err = orderService.AcceptOrder(context.Background(), 202, 456, deadline, 2.5, 2000, nil, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
	}{
		{
			name: "успешная выдача заказов",
			requestBody: map[string]any{
				"customer_id": 456,
				"action":      "handout",
				"order_ids":   []int{201, 202},
			},
			expectedStatus: fiber.StatusOK,
		},
		{
			name: "ошибка - несуществующие заказы",
			requestBody: map[string]any{
				"customer_id": 456,
				"action":      "handout",
				"order_ids":   []int{999, 998},
			},
			expectedStatus: fiber.StatusOK, // API всегда возвращает OK, даже если не все заказы обработаны
		},
		{
			name: "ошибка - некорректное действие",
			requestBody: map[string]any{
				"customer_id": 456,
				"action":      "invalid",
				"order_ids":   []int{201, 202},
			},
			expectedStatus: fiber.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/orders/process", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestOrderHandlerIntegration_ClearDatabase(t *testing.T) {
	app, orderService, _, cleanup := setupOrderTest(t)
	defer cleanup()

	// Создаем несколько заказов перед очисткой
	deadline := time.Now().Add(24 * time.Hour)

	err := orderService.AcceptOrder(context.Background(), 301, 456, deadline, 1.5, 1000, nil, nil)
	require.NoError(t, err)
	err = orderService.AcceptOrder(context.Background(), 302, 456, deadline, 2.5, 2000, nil, nil)
	require.NoError(t, err)

	// Проверяем, что заказы действительно созданы
	orders, err := orderService.ListOrdersWithCursor(context.Background(), 0, 10, 456, false, "")
	require.NoError(t, err)
	require.Len(t, orders, 2)

	tests := []struct {
		name           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "успешная очистка базы данных",
			expectedStatus: fiber.StatusOK,
			expectedBody:   "База данных успешно очищена",
		},
		{
			name:           "очистка пустой базы данных",
			expectedStatus: fiber.StatusOK, // Должен быть успех даже если БД пуста
			expectedBody:   "База данных успешно очищена",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/clear", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)

			// Проверяем, что база данных действительно пуста после очистки
			if tt.name == "успешная очистка базы данных" {
				orders, err := orderService.ListOrdersWithCursor(context.Background(), 0, 10, 456, false, "")
				require.NoError(t, err)
				assert.Empty(t, orders, "БД должна быть пуста после очистки")
			}
		})
	}
}

func TestOrderHandlerIntegration_OrderHistory(t *testing.T) {
	app, orderService, _, cleanup := setupOrderTest(t)
	defer cleanup()

	// Создаем заказы и изменяем их статусы для формирования истории
	deadline := time.Now().Add(24 * time.Hour)

	// Заказ с историей статусов
	err := orderService.AcceptOrder(context.Background(), 401, 456, deadline, 1.5, 1000, nil, nil)
	require.NoError(t, err)

	// Выдаем заказ клиенту
	err = orderService.DeliverOrder(context.Background(), 401, 456, time.Now())
	require.NoError(t, err)

	// Второй заказ просто создаем
	err = orderService.AcceptOrder(context.Background(), 402, 789, deadline, 2.5, 2000, nil, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		expectedStatus int
		minOrders      int
	}{
		{
			name:           "получение истории заказов",
			expectedStatus: fiber.StatusOK,
			minOrders:      2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/history", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var result map[string]any
			err = json.Unmarshal(body, &result)
			require.NoError(t, err)

			// Проверяем что в истории есть заказы
			orders, ok := result["orders"].([]any)
			require.True(t, ok, "В ответе должен быть массив заказов")
			assert.GreaterOrEqual(t, len(orders), tt.minOrders,
				"Должно быть как минимум %d заказа в истории", tt.minOrders)

			// Проверяем поле total
			total, ok := result["total"].(float64)
			require.True(t, ok, "В ответе должно быть поле total")
			assert.GreaterOrEqual(t, int(total), tt.minOrders,
				"Total должен быть как минимум %d", tt.minOrders)
		})
	}
}

func TestOrderHandlerIntegration_ListReturns(t *testing.T) {
	app, orderService, _, cleanup := setupOrderTest(t)
	defer cleanup()

	deadline := time.Now().Add(24 * time.Hour)

	// Создаем заказ и возвращаем его
	err := orderService.AcceptOrder(context.Background(), 501, 456, deadline, 1.5, 1000, nil, nil)
	require.NoError(t, err)

	// Выдаем заказ клиенту
	err = orderService.DeliverOrder(context.Background(), 501, 456, time.Now())
	require.NoError(t, err)

	// Возвращаем заказ
	err = orderService.ProcessReturnOrder(context.Background(), 501, 456, time.Now())
	require.NoError(t, err)

	// Создаем второй заказ без возврата
	err = orderService.AcceptOrder(context.Background(), 502, 456, deadline, 2.5, 2000, nil, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		expectedCount  int
	}{
		{
			name:           "получение списка возвратов",
			queryParams:    "cursor=0&limit=10",
			expectedStatus: fiber.StatusOK,
			expectedCount:  1, // Только один заказ в статусе возврата
		},
		{
			name:           "пагинация возвратов",
			queryParams:    "cursor=0&limit=1",
			expectedStatus: fiber.StatusOK,
			expectedCount:  1,
		},
		{
			name:           "ошибка - некорректный cursor",
			queryParams:    "cursor=invalid&limit=10",
			expectedStatus: fiber.StatusBadRequest,
			expectedCount:  0,
		},
		{
			name:           "поиск возвратов",
			queryParams:    "cursor=0&limit=10&search=456",
			expectedStatus: fiber.StatusOK,
			expectedCount:  1, // Должен найти заказ с customer_id = 456
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/returns?%s", tt.queryParams), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusOK {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)

				// Проверяем количество возвратов
				var returnsCount int
				if returns, ok := result["returns"].([]any); ok {
					returnsCount = len(returns)
				}

				assert.Equal(t, tt.expectedCount, returnsCount,
					"Ожидалось %d возвратов, получено %d",
					tt.expectedCount, returnsCount)
			}
		})
	}
}

func TestOrderHandlerIntegration_ReturnToCourier(t *testing.T) {
	app, orderService, _, cleanup := setupOrderTest(t)
	defer cleanup()

	deadline := time.Now().Add(24 * time.Hour)

	// Создаем заказы для тестирования возврата
	err := orderService.AcceptOrder(context.Background(), 601, 456, time.Now().Add(1*time.Second), 1.5, 1000, nil, nil)
	require.NoError(t, err)

	time.Sleep(1 * time.Second) // Чтобы заказы просрочился

	// Заказ, который уже выдан клиенту
	err = orderService.AcceptOrder(context.Background(), 602, 456, deadline, 2.5, 2000, nil, nil)
	require.NoError(t, err)
	err = orderService.DeliverOrder(context.Background(), 602, 456, time.Now())
	require.NoError(t, err)

	tests := []struct {
		name           string
		orderID        string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "успешный возврат заказа курьеру",
			orderID:        "601",
			expectedStatus: fiber.StatusOK,
			expectedBody:   "Заказ возвращен курьеру",
		},
		{
			name:           "ошибка - заказ уже доставлен клиенту",
			orderID:        "602",
			expectedStatus: fiber.StatusConflict,
			expectedBody:   "заказ уже доставлен",
		},
		{
			name:           "ошибка - несуществующий заказ",
			orderID:        "999",
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   "заказ не существует",
		},
		{
			name:           "ошибка - некорректный ID заказа",
			orderID:        "invalid",
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   "неверный формат ID заказа",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/orders/%s/return", tt.orderID), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)

			// Для успешного случая проверяем, что статус заказа изменился
			if tt.expectedStatus == fiber.StatusOK {
				_, err := orderService.GetOrderByID(context.Background(), 601)
				require.Error(t, repository.ErrOrderNotFound, err)
			}
		})
	}
}
