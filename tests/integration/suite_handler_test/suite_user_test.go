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

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/suite"
	"gitlab.ozon.dev/gojhw1/pkg/handler"
	"gitlab.ozon.dev/gojhw1/pkg/model"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
)

// UserHandlerSuite содержит тесты для хендлеров пользователей
type UserHandlerSuite struct {
	BaseSuite
	userRepo *repository.PostgresUserRepository
}

// SetupSuite настраивает окружение для тестов пользовательских хэндлеров
func (s *UserHandlerSuite) SetupSuite() {
	s.setupTestDB()

	// Создаём репозиторий
	s.userRepo = repository.NewPostgresUserRepository(s.pool)

	// Создаём хэндлер
	userHandler := handler.NewUserHandler(s.userRepo)

	// Создаём приложение Fiber
	s.app = fiber.New()

	// Регистрируем маршруты
	s.app.Post("/users/register", userHandler.CreateUser)
	s.app.Get("/users", userHandler.ListUsers)
	s.app.Get("/users/:id", userHandler.GetUser)
	s.app.Put("/users/:id", userHandler.UpdateUser)
	s.app.Delete("/users/:id", userHandler.DeleteUser)
	s.app.Put("/users/:id/password", userHandler.UpdatePassword)
}

// TestCreateUser тестирует создание пользователя
func (s *UserHandlerSuite) TestCreateUser() {
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
		s.Run(tt.name, func() {
			reqBody, err := json.Marshal(tt.requestBody)
			s.Require().NoError(err)

			req := httptest.NewRequest(http.MethodPost, "/users/register", bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)

			s.Contains(string(body), tt.expectedBody)
		})
	}
}

// TestListUsers тестирует получение списка пользователей
func (s *UserHandlerSuite) TestListUsers() {
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
		s.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodGet, "/users", nil)

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)

			s.Contains(string(body), tt.expectedBody)
		})
	}
}

// TestGetUser тестирует получение информации о пользователе
func (s *UserHandlerSuite) TestGetUser() {
	// Создаем пользователя для тестирования
	err := s.userRepo.Create(context.Background(), model.User{
		Username: "testuser",
		Role:     "user",
	}, "testpass")
	s.Require().NoError(err)

	// Получаем созданного пользователя для определения его ID
	users, err := s.userRepo.List(context.Background(), "")
	s.Require().NoError(err)
	s.NotEmpty(users)

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
		s.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/users/%s", tt.userID), nil)

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == fiber.StatusOK {
				body, err := io.ReadAll(resp.Body)
				s.Require().NoError(err)

				var result map[string]any
				err = json.Unmarshal(body, &result)
				s.Require().NoError(err)

				s.Equal("testuser", result["username"])
				s.Equal("user", result["role"])
			}
		})
	}
}

// TestUpdateUser тестирует обновление информации о пользователе
func (s *UserHandlerSuite) TestUpdateUser() {
	err := s.userRepo.Create(context.Background(), model.User{
		Username: "testuser",
		Role:     "user",
	}, "testpass")
	s.Require().NoError(err)

	users, err := s.userRepo.List(context.Background(), "")
	s.Require().NoError(err)
	s.NotEmpty(users)

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
		s.Run(tt.name, func() {
			reqBody, err := json.Marshal(tt.requestBody)
			s.Require().NoError(err)

			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/users/%s", tt.userID), bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)

			s.Contains(string(body), tt.expectedBody)
		})
	}
}

// TestUpdatePassword тестирует обновление пароля пользователя
func (s *UserHandlerSuite) TestUpdatePassword() {
	// Создаем пользователя для тестирования
	err := s.userRepo.Create(context.Background(), model.User{
		Username: "passworduser",
		Role:     "user",
	}, "oldpassword")
	s.Require().NoError(err)

	// Получаем созданного пользователя для определения его ID
	users, err := s.userRepo.List(context.Background(), "")
	s.Require().NoError(err)

	var userID int64
	for _, u := range users {
		if u.Username == "passworduser" {
			userID = u.ID
			break
		}
	}
	s.NotZero(userID)

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
		s.Run(tt.name, func() {
			reqBody, err := json.Marshal(tt.requestBody)
			s.Require().NoError(err)

			req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/users/%s/password", tt.userID), bytes.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)

			s.Contains(string(body), tt.expectedBody)
		})
	}
}

// TestDeleteUser тестирует удаление пользователя
func (s *UserHandlerSuite) TestDeleteUser() {
	err := s.userRepo.Create(context.Background(), model.User{
		Username: "testuser",
		Role:     "user",
	}, "testpass")
	s.Require().NoError(err)

	users, err := s.userRepo.List(context.Background(), "")
	s.Require().NoError(err)
	s.NotEmpty(users)

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
		s.Run(tt.name, func() {
			req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/users/%s", tt.userID), nil)

			resp, err := s.app.Test(req)
			s.Require().NoError(err)

			s.Equal(tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			s.Require().NoError(err)

			s.Contains(string(body), tt.expectedBody)
		})
	}
}

func TestUserHandlerSuite(t *testing.T) {
	suite.Run(t, new(UserHandlerSuite))
}
