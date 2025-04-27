package router

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestInitFiberApp(t *testing.T) {
	// Создаем контроллер для моков
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Создаем моки необходимых интерфейсов
	mockOrderService := NewMockorderServiceInterface(ctrl)
	mockUserRepo := NewMockuserRepository(ctrl)
	mockAuditLogger := NewMockauditLoggerInterface(ctrl)

	// Настраиваем ожидаемое поведение для аутентификации
	mockUserRepo.EXPECT().
		CheckPassword(gomock.Any(), "testuser", "testpass").
		Return(true).
		AnyTimes()

	mockUserRepo.EXPECT().
		List(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	mockOrderService.EXPECT().
		ListReturnsWithCursor(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	mockOrderService.EXPECT().
		ListOrdersWithCursor(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	mockAuditLogger.EXPECT().
		Log(gomock.Any(), gomock.Any()).
		Return().
		AnyTimes()

	// Инициализируем приложение
	ctx := context.Background()
	app := InitFiberApp(ctx, mockOrderService, mockUserRepo, mockAuditLogger)

	// Проверяем незащищенные маршруты
	t.Run("Public routes", func(t *testing.T) {
		// Тест на регистрацию пользователя
		req := httptest.NewRequest(fiber.MethodPost, "/api/v1/users/register", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.NotEqual(t, fiber.StatusUnauthorized, resp.StatusCode)
	})

	// Проверяем защищенные маршруты
	t.Run("Protected routes with auth", func(t *testing.T) {
		tests := []struct {
			name   string
			path   string
			method string
		}{
			{
				name:   "get users",
				path:   "/api/v1/users",
				method: fiber.MethodGet,
			},
			{
				name:   "get orders",
				path:   "/api/v1/orders",
				method: fiber.MethodGet,
			},
			{
				name:   "get returns",
				path:   "/api/v1/returns",
				method: fiber.MethodGet,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := httptest.NewRequest(tt.method, tt.path, nil)
				// Добавляем заголовок Basic Auth
				req.SetBasicAuth("testuser", "testpass")

				resp, err := app.Test(req, -1) // Используем -1, чтобы избежать таймаута
				require.NoError(t, err)

				// Проверяем, что авторизация работает (не 401)
				assert.NotEqual(t, fiber.StatusUnauthorized, resp.StatusCode)
			})
		}
	})

	// Проверяем защищенные маршруты без аутентификации
	t.Run("Protected routes without auth", func(t *testing.T) {
		paths := []string{
			"/api/v1/users",
			"/api/v1/orders",
			"/api/v1/returns",
		}

		for _, path := range paths {
			t.Run(path, func(t *testing.T) {
				req := httptest.NewRequest(fiber.MethodGet, path, nil)
				// Не добавляем заголовок Basic Auth

				resp, err := app.Test(req, -1) // Используем -1, чтобы избежать таймаута
				require.NoError(t, err)

				// Проверяем, что получаем 401 для защищенных маршрутов
				assert.Equal(t, fiber.StatusUnauthorized, resp.StatusCode)
			})
		}
	})
}
