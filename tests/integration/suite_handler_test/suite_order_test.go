package suite_handler_test

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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gitlab.ozon.dev/gojhw1/pkg/handler"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
	"gitlab.ozon.dev/gojhw1/pkg/service"
	"gitlab.ozon.dev/gojhw1/pkg/utils"
)

// OrderHandlerSuite содержит тесты для хендлеров заказов
type OrderHandlerSuite struct {
	BaseSuite
	orderService *service.OrderService
	orderRepo    *repository.PostgresOrderRepository
	auditRepo    *repository.PostgresAuditRepository
	logger       *utils.AuditLogger
}

// SetupSuite настраивает окружение для тестов заказов
func (s *OrderHandlerSuite) SetupSuite() {
	s.setupTestDB()
	s.setupTestRedis()

	s.auditRepo = repository.NewPostgresAuditRepository(s.pool)

	s.logger = utils.NewAuditLogger(context.Background(), s.auditRepo, 2, 5, 500*time.Millisecond)

	// Создаём репозитории
	s.orderRepo = repository.NewPostgresOrderRepository(s.pool)

	// Создаём сервис
	s.orderService = service.NewOrderService(s.orderRepo, s.logger, s.redisCache)

	// Создаём хэндлер
	orderHandler := handler.NewOrderHandler(s.orderService)

	// Создаём приложение Fiber
	s.app = fiber.New()

	// Регистрируем маршруты
	s.app.Post("/orders", orderHandler.CreateOrder)
	s.app.Get("/orders/:id", orderHandler.GetOrder)
	s.app.Post("/orders/:id/return", orderHandler.ReturnToCourier)
	s.app.Post("/orders/process", orderHandler.ProcessCustomer)
	s.app.Get("/orders", orderHandler.ListOrders)
	s.app.Get("/returns", orderHandler.ListReturns)
	s.app.Get("/history", orderHandler.OrderHistory)
	s.app.Delete("/clear", orderHandler.ClearDatabase)
}

func (s *OrderHandlerSuite) TearDownSuite() {
	s.logger.Shutdown()
}

// TestCreateOrder тестирует создание заказа
func (s *OrderHandlerSuite) TestCreateOrder() {
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
		s.Run(tt.name, func() {
			reqBody, err := json.Marshal(tt.requestBody)
			s.Require().NoError(err)

			req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusCreated {
				body, err := io.ReadAll(resp.Body)
				s.Require().NoError(err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				s.Require().NoError(err)

				s.Equal(float64(123), result["id"])
				s.Equal(float64(456), result["customer_id"])
			}
		})
	}
}

// TestGetOrder тестирует получение заказа по ID
func (s *OrderHandlerSuite) TestGetOrder() {
	deadline := time.Now().Add(24 * time.Hour)
	err := s.orderService.AcceptOrder(context.Background(), 123, 456, deadline, 1.5, 1000, nil, nil)
	s.Require().NoError(err)

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
		s.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders/%s", tt.orderID), nil)

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusOK {
				body, err := io.ReadAll(resp.Body)
				s.Require().NoError(err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				s.Require().NoError(err)

				s.Equal(float64(123), result["id"])
				s.Equal(float64(456), result["customer_id"])
				s.Equal(float64(1.5), result["weight"])
				s.Equal(float64(1000), result["cost"])
			}
		})
	}
}

// TestListOrders тестирует получение списка заказов
func (s *OrderHandlerSuite) TestListOrders() {
	deadline := time.Now().Add(24 * time.Hour)

	// Заказы для клиента 456
	err := s.orderService.AcceptOrder(context.Background(), 101, 456, deadline, 1.5, 1000, nil, nil)
	s.Require().NoError(err)
	err = s.orderService.AcceptOrder(context.Background(), 102, 456, deadline, 2.5, 2000, nil, nil)
	s.Require().NoError(err)

	// Заказ для другого клиента
	err = s.orderService.AcceptOrder(context.Background(), 103, 789, deadline, 3.5, 3000, nil, nil)
	s.Require().NoError(err)

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
		s.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders?%s", tt.queryParams), nil)

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusOK {
				body, err := io.ReadAll(resp.Body)
				s.Require().NoError(err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				s.Require().NoError(err)

				// Проверяем поле orders безопасно, учитывая возможность nil
				var ordersCount int
				if orders, ok := result["orders"].([]any); ok && orders != nil {
					ordersCount = len(orders)
				}

				s.Equal(tt.expectedCount, ordersCount,
					"Ожидалось %d заказов, получено %d. Тело ответа: %s",
					tt.expectedCount, ordersCount, string(body))
			}
		})
	}
}

// TestProcessCustomer тестирует обработку заказов клиента
func (s *OrderHandlerSuite) TestProcessCustomer() {
	deadline := time.Now().Add(24 * time.Hour)

	err := s.orderService.AcceptOrder(context.Background(), 201, 456, deadline, 1.5, 1000, nil, nil)
	s.Require().NoError(err)

	err = s.orderService.AcceptOrder(context.Background(), 202, 456, deadline, 2.5, 2000, nil, nil)
	s.Require().NoError(err)

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
		s.Run(tt.name, func() {
			reqBody, err := json.Marshal(tt.requestBody)
			s.Require().NoError(err)

			req := httptest.NewRequest(http.MethodPost, "/orders/process", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)
		})
	}
}

// TestClearDatabase тестирует очистку базы данных
func (s *OrderHandlerSuite) TestClearDatabase() {
	// Создаем несколько заказов перед очисткой
	deadline := time.Now().Add(24 * time.Hour)

	err := s.orderService.AcceptOrder(context.Background(), 301, 456, deadline, 1.5, 1000, nil, nil)
	s.Require().NoError(err)
	err = s.orderService.AcceptOrder(context.Background(), 302, 456, deadline, 2.5, 2000, nil, nil)
	s.Require().NoError(err)

	// Проверяем, что заказы действительно созданы
	orders, err := s.orderService.ListOrdersWithCursor(context.Background(), 0, 10, 456, false, "")
	s.Require().NoError(err)
	require.Len(s.T(), orders, 2)

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
		s.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodDelete, "/clear", nil)

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)

			s.Contains(string(body), tt.expectedBody)

			// Проверяем, что база данных действительно пуста после очистки
			if tt.name == "успешная очистка базы данных" {
				orders, err := s.orderService.ListOrdersWithCursor(context.Background(), 0, 10, 456, false, "")
				s.Require().NoError(err)
				s.Empty(orders, "БД должна быть пуста после очистки")
			}
		})
	}
}

// TestOrderHistory тестирует получение истории заказов
func (s *OrderHandlerSuite) TestOrderHistory() {
	// Создаем заказы и изменяем их статусы для формирования истории
	deadline := time.Now().Add(24 * time.Hour)

	// Заказ с историей статусов
	err := s.orderService.AcceptOrder(context.Background(), 401, 456, deadline, 1.5, 1000, nil, nil)
	s.Require().NoError(err)

	// Выдаем заказ клиенту
	err = s.orderService.DeliverOrder(context.Background(), 401, 456, time.Now())
	s.Require().NoError(err)

	// Второй заказ просто создаем
	err = s.orderService.AcceptOrder(context.Background(), 402, 789, deadline, 2.5, 2000, nil, nil)
	s.Require().NoError(err)

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
		s.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodGet, "/history", nil)

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)

			var result map[string]any
			err = json.Unmarshal(body, &result)
			s.Require().NoError(err)

			// Проверяем что в истории есть заказы
			orders, ok := result["orders"].([]any)
			require.True(s.T(), ok, "В ответе должен быть массив заказов")
			s.GreaterOrEqual(len(orders), tt.minOrders,
				"Должно быть как минимум %d заказа в истории", tt.minOrders)

			// Проверяем поле total
			total, ok := result["total"].(float64)
			require.True(s.T(), ok, "В ответе должно быть поле total")
			s.GreaterOrEqual(int(total), tt.minOrders,
				"Total должен быть как минимум %d", tt.minOrders)
		})
	}
}

// TestListReturns тестирует получение списка возвратов
func (s *OrderHandlerSuite) TestListReturns() {
	deadline := time.Now().Add(24 * time.Hour)

	// Создаем заказ и возвращаем его
	err := s.orderService.AcceptOrder(context.Background(), 501, 456, deadline, 1.5, 1000, nil, nil)
	s.Require().NoError(err)

	// Выдаем заказ клиенту
	err = s.orderService.DeliverOrder(context.Background(), 501, 456, time.Now())
	s.Require().NoError(err)

	// Возвращаем заказ
	err = s.orderService.ProcessReturnOrder(context.Background(), 501, 456, time.Now())
	s.Require().NoError(err)

	// Создаем второй заказ без возврата
	err = s.orderService.AcceptOrder(context.Background(), 502, 456, deadline, 2.5, 2000, nil, nil)
	s.Require().NoError(err)

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
		s.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/returns?%s", tt.queryParams), nil)

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusOK {
				body, err := io.ReadAll(resp.Body)
				s.Require().NoError(err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				s.Require().NoError(err)

				// Проверяем количество возвратов
				var returnsCount int
				if returns, ok := result["returns"].([]any); ok {
					returnsCount = len(returns)
				}

				s.Equal(tt.expectedCount, returnsCount,
					"Ожидалось %d возвратов, получено %d",
					tt.expectedCount, returnsCount)
			}
		})
	}
}

// TestReturnToCourier тестирует возврат заказа курьеру
func (s *OrderHandlerSuite) TestReturnToCourier() {
	deadline := time.Now().Add(24 * time.Hour)

	// Создаем заказы для тестирования возврата
	err := s.orderService.AcceptOrder(context.Background(), 601, 456, time.Now().Add(1*time.Second), 1.5, 1000, nil, nil)
	s.Require().NoError(err)

	time.Sleep(1 * time.Second) // Чтобы заказ просрочился

	// Заказ, который уже выдан клиенту
	err = s.orderService.AcceptOrder(context.Background(), 602, 456, deadline, 2.5, 2000, nil, nil)
	s.Require().NoError(err)
	err = s.orderService.DeliverOrder(context.Background(), 602, 456, time.Now())
	s.Require().NoError(err)

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
		s.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/orders/%s/return", tt.orderID), nil)

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)

			s.Contains(string(body), tt.expectedBody)

			// Для успешного случая проверяем, что статус заказа изменился
			if tt.expectedStatus == fiber.StatusOK {
				_, err := s.orderService.GetOrderByID(context.Background(), 601)
				require.Error(s.T(), repository.ErrOrderNotFound, err)
			}
		})
	}
}

// Добавляем функцию запуска тестов
func TestOrderHandlerSuite(t *testing.T) {
	suite.Run(t, new(OrderHandlerSuite))
}
