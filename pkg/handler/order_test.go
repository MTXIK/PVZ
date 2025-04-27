package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.ozon.dev/gojhw1/pkg/model"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
	"gitlab.ozon.dev/gojhw1/pkg/service"
	"go.uber.org/mock/gomock"
)

// setupOrderTest создает тестовое окружение и возвращает app, mockService и функцию для очистки ресурсов
func setupOrderTest(t *testing.T) (*fiber.App, *MockorderServiceInterface, func()) {
	ctrl := gomock.NewController(t)
	mockService := NewMockorderServiceInterface(ctrl)

	app := fiber.New()
	handler := NewOrderHandler(mockService)

	// Регистрация маршрутов для тестирования
	app.Post("/orders", handler.CreateOrder)
	app.Get("/orders/:id", handler.GetOrder)
	app.Post("/orders/:id/return", handler.ReturnToCourier)
	app.Post("/orders/process", handler.ProcessCustomer)
	app.Get("/orders", handler.ListOrders)
	app.Get("/returns", handler.ListReturns)
	app.Get("/history", handler.OrderHistory)
	app.Post("/upload", handler.AcceptOrdersFromFile)
	app.Delete("/clear", handler.ClearDatabase)

	cleanup := func() {
		ctrl.Finish()
	}

	return app, mockService, cleanup
}

func TestOrderHandler_CreateOrder(t *testing.T) {
	t.Parallel()
	// Структура тестовых случаев
	tests := []struct {
		name           string
		requestBody    any
		mockSetup      func(mockService *MockorderServiceInterface)
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "success creating order",
			requestBody: orderRequest{
				ID:          123,
				CustomerID:  456,
				DeadlineAt:  time.Now().Add(24 * time.Hour).Format(timeLayout),
				Weight:      1.5,
				Cost:        1000,
				PackageType: string(model.PackageBox),
				Wrapper:     string(model.WrapperFilm),
			},
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					AcceptOrder(gomock.Any(), int64(123), int64(456), gomock.Any(),
						float64(1.5), float64(1000), gomock.Any(), gomock.Any()).
					Return(nil)

				mockService.EXPECT().
					GetOrderByID(gomock.Any(), int64(123)).
					Return(model.Order{
						ID:         123,
						CustomerID: 456,
						Weight:     1.5,
						Cost:       1000,
					}, nil)
			},
			expectedStatus: fiber.StatusCreated,
			expectedBody:   `{"id":123,"customer_id":456}`,
		},
		{
			name: "validation error - invalid weight",
			requestBody: orderRequest{
				ID:         123,
				CustomerID: 456,
				DeadlineAt: time.Now().Add(24 * time.Hour).Format(timeLayout),
				Weight:     -1.5,
				Cost:       1000,
			},
			mockSetup:      func(mockService *MockorderServiceInterface) {},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"вес должен быть больше 0"}`,
		},
		{
			name: "error creating order",
			requestBody: orderRequest{
				ID:         123,
				CustomerID: 456,
				DeadlineAt: time.Now().Add(24 * time.Hour).Format(timeLayout),
				Weight:     1.5,
				Cost:       1000,
			},
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					AcceptOrder(gomock.Any(), int64(123), int64(456), gomock.Any(),
						float64(1.5), float64(1000), nil, nil).
					Return(service.ErrOrderExists)
			},
			expectedStatus: fiber.StatusConflict,
			expectedBody:   `{"error":"Ошибка при принятии заказа: заказ уже существует"}`,
		},
		{
			name: "error getting order by ID",
			requestBody: orderRequest{
				ID:         123,
				CustomerID: 456,
				DeadlineAt: time.Now().Add(24 * time.Hour).Format(timeLayout),
				Weight:     1.5,
				Cost:       1000,
			},
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					AcceptOrder(gomock.Any(), int64(123), int64(456), gomock.Any(),
						float64(1.5), float64(1000), gomock.Any(), gomock.Any()).
					Return(nil)

				mockService.EXPECT().
					GetOrderByID(gomock.Any(), int64(123)).
					Return(model.Order{}, repository.ErrOrderNotFound)
			},
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Ошибка при попытке получить сохраненный заказ: заказ не существует"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, mockService, cleanup := setupOrderTest(t)
			defer cleanup()

			// Настраиваем ожидаемое поведение мок-сервиса
			tt.mockSetup(mockService)

			// Подготавливаем тело запроса
			reqBody, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			// Создаем тестовый запрос
			req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			// Выполняем запрос
			resp, err := app.Test(req)
			require.NoError(t, err)

			// Проверяем статус ответа
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			// Читаем тело ответа
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Для более точной проверки JSON используем структуры вместо строкового сравнения
			if tt.expectedStatus == fiber.StatusCreated {
				// Для успешного случая проверяем конкретные поля
				var result map[string]any
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)
				assert.Equal(t, float64(123), result["id"])
				assert.Equal(t, float64(456), result["customer_id"])
			} else {
				// Для ошибок используем проверку на включение строки
				assert.Contains(t, string(body), tt.expectedBody)
			}
		})
	}
}

func TestOrderHandler_ReturnToCourier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		orderID        string
		mockSetup      func(mockService *MockorderServiceInterface)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:    "success returning order to courier",
			orderID: "123",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ReturnOrderToCourier(gomock.Any(), int64(123)).
					Return(nil)
			},
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"message":"Заказ возвращен курьеру 123"}`,
		},
		{
			name:    "error returning order to courier",
			orderID: "123",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ReturnOrderToCourier(gomock.Any(), int64(123)).
					Return(service.ErrOrderAlreadyDelivered)
			},
			expectedStatus: fiber.StatusConflict,
			expectedBody:   `{"error":"Ошибка при возврате заказа курьеру: заказ уже доставлен клиенту, возврат невозможен"}`,
		},
		{
			name:           "validation error - invalid order ID",
			orderID:        "abc",
			mockSetup:      func(mockService *MockorderServiceInterface) {},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"неверный формат ID заказа"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, mockService, cleanup := setupOrderTest(t)
			defer cleanup()

			tt.mockSetup(mockService)

			req := httptest.NewRequest(http.MethodPost, "/orders/"+tt.orderID+"/return", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestOrderHandler_ProcessCustomer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		requestBody    any
		mockSetup      func(mockService *MockorderServiceInterface)
		expectedStatus int
	}{
		{
			name: "success handout customer",
			requestBody: processRequest{
				CustomerID: 456,
				Action:     "handout",
				OrderIDs:   []int64{123, 124},
			},
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					DeliverOrder(gomock.Any(), int64(123), int64(456), gomock.Any()).
					Return(nil)

				mockService.EXPECT().
					DeliverOrder(gomock.Any(), int64(124), int64(456), gomock.Any()).
					Return(nil)
			},
			expectedStatus: fiber.StatusOK,
		},
		{
			name: "success return customer",
			requestBody: processRequest{
				CustomerID: 456,
				Action:     "return",
				OrderIDs:   []int64{123, 124},
			},
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ProcessReturnOrder(gomock.Any(), int64(123), int64(456), gomock.Any()).
					Return(nil)

				mockService.EXPECT().
					ProcessReturnOrder(gomock.Any(), int64(124), int64(456), gomock.Any()).
					Return(nil)
			},
			expectedStatus: fiber.StatusOK,
		},
		{
			name: "validation error - invalid action",
			requestBody: processRequest{
				CustomerID: 1,
				Action:     "invalid",
				OrderIDs:   []int64{123, 124},
			},
			mockSetup:      func(mockService *MockorderServiceInterface) {},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name: "error handout customer",
			requestBody: processRequest{
				CustomerID: 456,
				Action:     "handout",
				OrderIDs:   []int64{123, 124},
			},
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					DeliverOrder(gomock.Any(), int64(123), int64(456), gomock.Any()).
					Return(nil)

				mockService.EXPECT().
					DeliverOrder(gomock.Any(), int64(124), int64(456), gomock.Any()).
					Return(service.ErrWrongCustomer)
			},
			expectedStatus: fiber.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, mockService, cleanup := setupOrderTest(t)
			defer cleanup()

			tt.mockSetup(mockService)

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

func TestOrderHandler_ListOrders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(mockService *MockorderServiceInterface)
		expextedStatus int
	}{
		{
			name:        "success list orders",
			queryParams: "customer_id=456&cursor=0&limit=2",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ListOrdersWithCursor(gomock.Any(), int64(0), 3, int64(456), false, "").
					Return([]model.Order{
						{ID: 1, CustomerID: 456, Cost: 100},
						{ID: 2, CustomerID: 456, Cost: 200},
						{ID: 3, CustomerID: 456, Cost: 300},
					}, nil)
			},
			expextedStatus: fiber.StatusOK,
		},
		{
			name:        "error list orders",
			queryParams: "customer_id=456&cursor=0&limit=2",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ListOrdersWithCursor(gomock.Any(), int64(0), 3, int64(456), false, "").
					Return(nil, errors.New("error"))
			},
			expextedStatus: fiber.StatusInternalServerError,
		},
		{
			name:           "validation error - invalid customer ID",
			queryParams:    "customer_id=invalid&cursor=0&limit=2",
			mockSetup:      func(mockService *MockorderServiceInterface) {},
			expextedStatus: fiber.StatusBadRequest,
		},
		{
			name:           "validation error - invalid cursor",
			queryParams:    "customer_id=456&cursor=invalid&limit=2",
			mockSetup:      func(mockService *MockorderServiceInterface) {},
			expextedStatus: fiber.StatusBadRequest,
		},
		{
			name:           "validation error - invalid limit",
			queryParams:    "customer_id=456&cursor=0&limit=invalid",
			mockSetup:      func(mockService *MockorderServiceInterface) {},
			expextedStatus: fiber.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, mockService, cleanup := setupOrderTest(t)
			defer cleanup()

			tt.mockSetup(mockService)

			req := httptest.NewRequest(http.MethodGet, "/orders?"+tt.queryParams, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expextedStatus, resp.StatusCode)
		})
	}
}

func TestOrderHandler_ListReturns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		queryParams    string
		mockSetup      func(mockService *MockorderServiceInterface)
		expextedStatus int
	}{
		{
			name:        "success list returns",
			queryParams: "cursor=0&limit=2",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ListReturnsWithCursor(gomock.Any(), int64(0), 3, "").
					Return([]model.Order{
						{ID: 1, CustomerID: 456, Cost: 100, State: model.StateReturned},
						{ID: 2, CustomerID: 456, Cost: 200, State: model.StateReturned},
					}, nil)
			},
			expextedStatus: fiber.StatusOK,
		},
		{
			name:        "success list returns with search",
			queryParams: "cursor=0&limit=2&search=456",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ListReturnsWithCursor(gomock.Any(), int64(0), 3, "456").
					Return([]model.Order{
						{ID: 1, CustomerID: 456, Cost: 100, State: model.StateReturned},
					}, nil)
			},
			expextedStatus: fiber.StatusOK,
		},
		{
			name:        "error list returns",
			queryParams: "cursor=0&limit=2",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ListReturnsWithCursor(gomock.Any(), int64(0), 3, "").
					Return(nil, errors.New("error"))
			},
			expextedStatus: fiber.StatusInternalServerError,
		},
		{
			name:           "validation error - invalid cursor",
			queryParams:    "cursor=invalid&limit=2",
			mockSetup:      func(mockService *MockorderServiceInterface) {},
			expextedStatus: fiber.StatusBadRequest,
		},
		{
			name:           "validation error - invalid limit",
			queryParams:    "cursor=0&limit=invalid",
			mockSetup:      func(mockService *MockorderServiceInterface) {},
			expextedStatus: fiber.StatusBadRequest,
		},
		{
			name:        "has more returns",
			queryParams: "cursor=0&limit=2",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ListReturnsWithCursor(gomock.Any(), int64(0), 3, "").
					Return([]model.Order{
						{ID: 1, CustomerID: 456, Cost: 100, State: model.StateReturned},
						{ID: 2, CustomerID: 456, Cost: 200, State: model.StateReturned},
						{ID: 3, CustomerID: 456, Cost: 300, State: model.StateReturned},
					}, nil)
			},
			expextedStatus: fiber.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, mockService, cleanup := setupOrderTest(t)
			defer cleanup()

			tt.mockSetup(mockService)

			req := httptest.NewRequest(http.MethodGet, "/returns?"+tt.queryParams, nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expextedStatus, resp.StatusCode)
		})
	}
}

func TestOrderHandler_OrderHistory(t *testing.T) {
	tests := []struct {
		name           string
		mockSetup      func(mockService *MockorderServiceInterface)
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "успешное получение истории заказов",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					OrderHistory(gomock.Any(), "").
					Return([]model.Order{
						{ID: 1, CustomerID: 456, Cost: 100},
						{ID: 2, CustomerID: 789, Cost: 200},
					}, nil)
			},
			expectedStatus: fiber.StatusOK,
			expectedBody:   `"total":2`,
		},
		{
			name: "ошибка получения истории заказов",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					OrderHistory(gomock.Any(), "").
					Return(nil, errors.New("ошибка базы данных"))
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedBody:   `"error":"Ошибка при получении истории заказов: ошибка базы данных"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, mockService, cleanup := setupOrderTest(t)
			defer cleanup()

			tt.mockSetup(mockService)

			req := httptest.NewRequest(http.MethodGet, "/history", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestOrderHandler_GetOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		orderID        string
		mockSetup      func(mockService *MockorderServiceInterface)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:    "success getting order by ID",
			orderID: "123",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					GetOrderByID(gomock.Any(), int64(123)).
					Return(model.Order{
						ID:         123,
						CustomerID: 456,
						Weight:     1.5,
						Cost:       1000,
					}, nil)
			},
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"id":123,"customer_id":456,"weight":1.5,"cost":1000}`,
		},
		{
			name:    "error getting order by ID",
			orderID: "123",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					GetOrderByID(gomock.Any(), int64(123)).
					Return(model.Order{}, repository.ErrOrderNotFound)
			},
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Ошибка при получении заказа: заказ не существует"}`,
		},
		{
			name:           "validation error - invalid order ID",
			orderID:        "abc",
			mockSetup:      func(mockService *MockorderServiceInterface) {},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"неверный формат ID заказа"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, mockService, cleanup := setupOrderTest(t)
			defer cleanup()

			tt.mockSetup(mockService)

			req := httptest.NewRequest(http.MethodGet, "/orders/"+tt.orderID, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.expectedStatus == fiber.StatusOK {
				var result map[string]any
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)
				assert.Equal(t, float64(123), result["id"])
				assert.Equal(t, float64(456), result["customer_id"])
				assert.Equal(t, 1.5, result["weight"])
				assert.Equal(t, 1000.0, result["cost"])
			} else {
				assert.Contains(t, string(body), tt.expectedBody)
			}
		})
	}
}

func TestOrderHandler_ClearDatabase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		mockSetup      func(mockService *MockorderServiceInterface)
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "Success clear database",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ClearDatabase(gomock.Any()).
					Return(nil)
			},
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"message":"База данных успешно очищена"}`,
		},
		{
			name: "Error clear database",
			mockSetup: func(mockService *MockorderServiceInterface) {
				mockService.EXPECT().
					ClearDatabase(gomock.Any()).
					Return(repository.ErrOrdersNotFound)
			},
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Ошибка при очистке базы данных: заказы не найдены"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			app, mockService, cleanup := setupOrderTest(t)
			defer cleanup()

			tt.mockSetup(mockService)

			req := httptest.NewRequest(http.MethodDelete, "/clear", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}
