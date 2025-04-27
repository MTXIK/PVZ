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

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

func TestUserHandlerIntegration_CreateUser(t *testing.T) {
	// Настройка тестового окружения
	app, _, _, cleanup := setupUserTest(t)
	defer cleanup()

	tests := []struct {
		name           string
		requestBody    map[string]any
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "успешное создание пользователя",
			requestBody: map[string]any{
				"username": "testuser",
				"password": "testpass",
				"role":     "user",
			},
			expectedStatus: fiber.StatusCreated,
			expectedBody:   `{"message":"Пользователь успешно создан"}`,
		},
		{
			name: "ошибка - пользователь с таким именем уже существует",
			requestBody: map[string]any{
				"username": "testuser",
				"password": "testpass",
				"role":     "user",
			},
			expectedStatus: fiber.StatusConflict,
			expectedBody:   `{"error":"Пользователь с таким именем уже существует"}`,
		},
		{
			name: "ошибка - пустое имя пользователя",
			requestBody: map[string]any{
				"username": "",
				"password": "testpass",
				"role":     "user",
			},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"имя пользователя не может быть пустым"}`,
		},
		{
			name: "ошибка - пустой пароль",
			requestBody: map[string]any{
				"username": "testuser2",
				"password": "",
				"role":     "user",
			},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"пароль не может быть пустым"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/users/register", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestUserHandlerIntegration_ListUsers(t *testing.T) {
	app, _, _, cleanup := setupUserTest(t)
	defer cleanup()

	tests := []struct {
		name           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "успешное получение списка пользователей",
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"total":0,"users":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/users", nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestUserHandlerIntegration_GetUser(t *testing.T) {
	// Настройка тестового окружения
	app, userRepo, _, cleanup := setupUserTest(t)
	defer cleanup()

	// Создаем пользователя для тестирования
	err := userRepo.Create(context.Background(), model.User{
		Username: "testuser",
		Role:     "user",
	}, "testpass")
	require.NoError(t, err)

	// Получаем созданного пользователя для определения его ID
	users, err := userRepo.List(context.Background(), "")
	require.NoError(t, err)
	require.NotEmpty(t, users)

	userID := users[0].ID

	tests := []struct {
		name           string
		userID         string
		expectedStatus int
	}{
		{
			name:           "успешное получение пользователя",
			userID:         fmt.Sprintf("%d", userID),
			expectedStatus: fiber.StatusOK,
		},
		{
			name:           "ошибка - несуществующий ID пользователя",
			userID:         "999999",
			expectedStatus: fiber.StatusNotFound,
		},
		{
			name:           "ошибка - некорректный ID пользователя",
			userID:         "invalid",
			expectedStatus: fiber.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/users/%s", tt.userID), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusOK {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Equal(t, "testuser", result["username"])
				assert.Equal(t, "user", result["role"])
			}
		})
	}
}

func TestUserHandlerIntegration_UpdateUser(t *testing.T) {
	app, userRepo, _, cleanup := setupUserTest(t)
	defer cleanup()

	err := userRepo.Create(context.Background(), model.User{
		Username: "testuser",
		Role:     "user",
	}, "testpass")
	require.NoError(t, err)

	users, err := userRepo.List(context.Background(), "")
	require.NoError(t, err)
	require.NotEmpty(t, users)

	userID := users[0].ID

	tests := []struct {
		name           string
		userID         string
		requestBody    map[string]any
		expectedStatus int
		expectedBody   string
	}{
		{
			name:   "успешное обновление пользователя",
			userID: fmt.Sprintf("%d", userID),
			requestBody: map[string]any{
				"username": "newuser",
				"role":     "admin",
			},
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"message":"Пользователь успешно обновлен"}`,
		},
		{
			name:   "ошибка - не существующий ID пользователя",
			userID: "999999",
			requestBody: map[string]any{
				"username": "",
				"role":     "",
			},
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Пользователь не найден"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/users/%s", tt.userID), bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestUserHandlerIntegration_UpdatePassword(t *testing.T) {
	// Настройка тестового окружения
	app, userRepo, _, cleanup := setupUserTest(t)
	defer cleanup()

	// Создаем пользователя для тестирования
	err := userRepo.Create(context.Background(), model.User{
		Username: "passworduser",
		Role:     "user",
	}, "oldpassword")
	require.NoError(t, err)

	// Получаем созданного пользователя для определения его ID
	users, err := userRepo.List(context.Background(), "")
	require.NoError(t, err)

	var userID int64
	for _, u := range users {
		if u.Username == "passworduser" {
			userID = u.ID
			break
		}
	}
	require.NotZero(t, userID)

	tests := []struct {
		name           string
		userID         string
		requestBody    map[string]string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:   "успешное обновление пароля",
			userID: fmt.Sprintf("%d", userID),
			requestBody: map[string]string{
				"password": "newpassword",
			},
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"message":"Пароль успешно обновлен"}`,
		},
		{
			name:   "ошибка - пустой пароль",
			userID: fmt.Sprintf("%d", userID),
			requestBody: map[string]string{
				"password": "",
			},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"пароль не может быть пустым"}`,
		},
		{
			name:   "ошибка - несуществующий ID пользователя",
			userID: "999999",
			requestBody: map[string]string{
				"password": "newpassword",
			},
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Пользователь не найден"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/users/%s/password", tt.userID), bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestUserHandlerIntegration_DeleteUser(t *testing.T) {
	app, userRepo, _, cleanup := setupUserTest(t)
	defer cleanup()

	err := userRepo.Create(context.Background(), model.User{
		Username: "testuser",
		Role:     "user",
	}, "testpass")
	require.NoError(t, err)

	users, err := userRepo.List(context.Background(), "")
	require.NoError(t, err)
	require.NotEmpty(t, users)

	userID := users[0].ID

	tests := []struct {
		name           string
		userID         string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "успешное удаление пользователя",
			userID:         fmt.Sprintf("%d", userID),
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"message":"Пользователь успешно удален"}`,
		},
		{
			name:           "ошибка - несуществующий ID пользователя",
			userID:         "999999",
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Пользователь не найден"}`,
		},
		{
			name:           "ошибка - неверный формат ID пользователя",
			userID:         "invalid",
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"неверный формат ID пользователя"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/users/%s", tt.userID), nil)

			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}
