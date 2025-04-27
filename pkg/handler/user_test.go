package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"go.uber.org/mock/gomock"
)

func setupUserTest(t *testing.T) (*fiber.App, *MockuserRepository, func()) {
	ctrl := gomock.NewController(t)
	mockDB := NewMockuserRepository(ctrl)

	app := fiber.New()
	handler := NewUserHandler(mockDB)

	app.Post("/users/register", handler.CreateUser)
	app.Get("/users", handler.ListUsers)
	app.Get("/users/:id", handler.GetUser)
	app.Put("/users/:id", handler.UpdateUser)
	app.Delete("/users/:id", handler.DeleteUser)
	app.Put("/users/:id/password", handler.UpdatePassword)

	cleanup := func() {
		ctrl.Finish()
	}

	return app, mockDB, cleanup
}

func TestUserHandler_CreateUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		requestBody    any
		mockSetup      func(mock *MockuserRepository)
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "success create user",
			requestBody: сreateUserRequest{
				Username: "testuser",
				Password: "testpass",
				Role:     "user",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().Create(gomock.Any(), model.User{
					Username: "testuser",
					Role:     "user",
				}, "testpass").Return(nil)
			},
			expectedStatus: fiber.StatusCreated,
			expectedBody:   `{"message":"Пользователь успешно создан"}`,
		},
		{
			name: "error create user - already exists",
			requestBody: сreateUserRequest{
				Username: "testuser",
				Password: "testpass",
				Role:     "user",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().Create(gomock.Any(), model.User{
					Username: "testuser",
					Role:     "user",
				}, "testpass").Return(repository.ErrUserAlreadyExists)
			},
			expectedStatus: fiber.StatusConflict,
			expectedBody:   `{"error":"Пользователь с таким именем уже существует"}`,
		},
		{
			name: "error create user - internal error",
			requestBody: сreateUserRequest{
				Username: "testuser",
				Password: "testpass",
				Role:     "user",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().Create(gomock.Any(), model.User{
					Username: "testuser",
					Role:     "user",
				}, "testpass").Return(errors.New("unexpected error"))
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedBody:   `{"error":"Ошибка при создании пользователя"}`,
		},
		{
			name: "validation error",
			requestBody: сreateUserRequest{
				Username: "",
				Password: "testpass",
				Role:     "user",
			},
			mockSetup:      func(mockDB *MockuserRepository) {},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"имя пользователя не может быть пустым"}`,
		},
		{
			name: "empty role",
			requestBody: сreateUserRequest{
				Username: "testuser",
				Password: "testpass",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().Create(gomock.Any(), model.User{
					Username: "testuser",
					Role:     "user",
				}, "testpass").Return(nil)
			},
			expectedStatus: fiber.StatusCreated,
			expectedBody:   `{"message":"Пользователь успешно создан"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, mockDB, cleanup := setupUserTest(t)
			defer cleanup()

			tt.mockSetup(mockDB)

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

func TestUserHandler_GetUser(t *testing.T) {
	t.Parallel()

	now := time.Now()

	testUser := model.User{
		ID:        1,
		Username:  "testuser",
		Role:      "user",
		CreatedAt: now,
		UpdatedAt: now,
	}

	tests := []struct {
		name           string
		userID         string
		mockSetup      func(mock *MockuserRepository)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:   "success get user",
			userID: "1",
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(testUser, nil)
			},
			expectedBody: fmt.Sprintf(`{"id":1,"username":"testuser","role":"user","created_at":"%s","updated_at":"%s"}`,
				testUser.CreatedAt.Format(time.RFC3339Nano),
				testUser.UpdatedAt.Format(time.RFC3339Nano)),
			expectedStatus: fiber.StatusOK,
		},
		{
			name:   "error get user - not found",
			userID: "1",
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{}, repository.ErrUserNotFound)
			},
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Пользователь не найден"}`,
		},
		{
			name:   "error get user - internal error",
			userID: "1",
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{}, errors.New("unexpected error"))
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedBody:   `{"error":"Ошибка при получении пользователя"}`,
		},
		{
			name:           "error get user - invalid id",
			userID:         "invalid",
			mockSetup:      func(mockDB *MockuserRepository) {},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"неверный формат ID пользователя"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, mockDB, cleanup := setupUserTest(t)
			defer cleanup()

			tt.mockSetup(mockDB)

			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/users/%s", tt.userID), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(body), tt.expectedBody)
		})
	}
}

func TestUserHandler_ListUsers(t *testing.T) {
	t.Parallel()

	now := time.Now()
	testUsers := []model.User{
		{
			ID:        1,
			Username:  "testuser1",
			Role:      "user",
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        2,
			Username:  "testuser2",
			Role:      "admin",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	tests := []struct {
		name           string
		mockSetup      func(mock *MockuserRepository)
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "success list users",
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().List(gomock.Any(), "").Return(testUsers, nil)
			},
			expectedStatus: fiber.StatusOK,
			expectedBody: fmt.Sprintf(`{"total":2,"users":[%s,%s]}`,
				fmt.Sprintf(`{"id":1,"username":"testuser1","role":"user","created_at":"%s","updated_at":"%s"}`,
					testUsers[0].CreatedAt.Format(time.RFC3339Nano),
					testUsers[0].UpdatedAt.Format(time.RFC3339Nano)),
				fmt.Sprintf(`{"id":2,"username":"testuser2","role":"admin","created_at":"%s","updated_at":"%s"}`,
					testUsers[1].CreatedAt.Format(time.RFC3339Nano),
					testUsers[1].UpdatedAt.Format(time.RFC3339Nano)),
			),
		},
		{
			name: "error list users - internal error",
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().List(gomock.Any(), "").Return(nil, errors.New("unexpected error"))
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedBody:   `{"error":"Ошибка при получении списка пользователей"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, mockDB, cleanup := setupUserTest(t)
			defer cleanup()

			tt.mockSetup(mockDB)

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

func TestUserHandler_UpdateUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userID         string
		requestBody    any
		mockSetup      func(mock *MockuserRepository)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:   "успешное обновление пользователя",
			userID: "1",
			requestBody: map[string]string{
				"username": "updated_user",
				"role":     "admin",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{
					ID:       1,
					Username: "old_username",
					Role:     "user",
				}, nil)
				mockDB.EXPECT().Update(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, user model.User) error {
						assert.Equal(t, int64(1), user.ID)
						assert.Equal(t, "updated_user", user.Username)
						assert.Equal(t, "admin", user.Role)
						return nil
					})
			},
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"message":"Пользователь успешно обновлен"}`,
		},
		{
			name:   "ошибка обновления - пользователь не найден",
			userID: "1",
			requestBody: map[string]string{
				"username": "updated_user",
				"role":     "admin",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{}, repository.ErrUserNotFound)
			},
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Пользователь не найден"}`,
		},
		{
			name:   "ошибка обновления - internal error",
			userID: "1",
			requestBody: map[string]string{
				"username": "updated_user",
				"role":     "admin",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{}, errors.New("internal error"))
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedBody:   `{"error":"Ошибка при получении пользователя"}`,
		},
		{
			name:   "ошибка обновления - внутренняя ошибка",
			userID: "1",
			requestBody: map[string]string{
				"username": "updated_user",
				"role":     "admin",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{
					ID:       1,
					Username: "old_username",
					Role:     "user",
				}, nil)
				mockDB.EXPECT().Update(gomock.Any(), gomock.Any()).Return(errors.New("внутренняя ошибка"))
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedBody:   `{"error":"Ошибка при обновлении пользователя"}`,
		},
		{
			name:           "ошибка валидации - неверный ID",
			userID:         "invalid",
			requestBody:    map[string]string{},
			mockSetup:      func(mockDB *MockuserRepository) {},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"неверный формат ID пользователя"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, mockDB, cleanup := setupUserTest(t)
			defer cleanup()

			tt.mockSetup(mockDB)

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

func TestUserHandler_UpdatePassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userID         string
		requestBody    any
		mockSetup      func(mock *MockuserRepository)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:   "успешное обновление пароля",
			userID: "1",
			requestBody: map[string]string{
				"password": "newpassword",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{
					ID:       1,
					Username: "user",
					Role:     "user",
				}, nil)
				mockDB.EXPECT().UpdatePassword(gomock.Any(), int64(1), "newpassword").Return(nil)
			},
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"message":"Пароль успешно обновлен"}`,
		},
		{
			name:   "ошибка обновления пароля - пользователь не найден при проверке",
			userID: "1",
			requestBody: map[string]string{
				"password": "newpassword",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{}, repository.ErrUserNotFound)
			},
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Пользователь не найден"}`,
		},
		{
			name:   "ошибка обновления пароля - ошибка при обновлении",
			userID: "1",
			requestBody: map[string]string{
				"password": "newpassword",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{
					ID:       1,
					Username: "user",
					Role:     "user",
				}, nil)
				mockDB.EXPECT().UpdatePassword(gomock.Any(), int64(1), "newpassword").Return(errors.New("внутренняя ошибка"))
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedBody:   `{"error":"Ошибка при обновлении пароля"}`,
		},
		{
			name:   "ошибка обновления пароля - internal error",
			userID: "1",
			requestBody: map[string]string{
				"password": "newpassword",
			},
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().GetByID(gomock.Any(), int64(1)).Return(model.User{}, errors.New("internal error"))
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedBody:   `{"error":"Ошибка при получении пользователя"}`,
		},
		{
			name:           "ошибка валидации - неверный ID",
			userID:         "invalid",
			requestBody:    map[string]string{},
			mockSetup:      func(mockDB *MockuserRepository) {},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"неверный формат ID пользователя"}`,
		},
		{
			name:   "ошибка валидации - пустой пароль",
			userID: "1",
			requestBody: map[string]string{
				"password": "",
			},
			mockSetup:      func(mockDB *MockuserRepository) {},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"пароль не может быть пустым"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, mockDB, cleanup := setupUserTest(t)
			defer cleanup()

			tt.mockSetup(mockDB)

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
func TestUserHandler_DeleteUser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		userID         string
		mockSetup      func(mock *MockuserRepository)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:   "success delete user",
			userID: "1",
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().Delete(gomock.Any(), int64(1)).Return(nil)
			},
			expectedStatus: fiber.StatusOK,
			expectedBody:   `{"message":"Пользователь успешно удален"}`,
		},
		{
			name:   "error delete user - not found",
			userID: "1",
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().Delete(gomock.Any(), int64(1)).Return(repository.ErrUserNotFound)
			},
			expectedStatus: fiber.StatusNotFound,
			expectedBody:   `{"error":"Пользователь не найден"}`,
		},
		{
			name:   "error delete user - internal error",
			userID: "1",
			mockSetup: func(mockDB *MockuserRepository) {
				mockDB.EXPECT().Delete(gomock.Any(), int64(1)).Return(errors.New("внутренняя ошибка"))
			},
			expectedStatus: fiber.StatusInternalServerError,
			expectedBody:   `{"error":"Ошибка при удалении пользователя"}`,
		},
		{
			name:           "validation error - invalid ID",
			userID:         "invalid",
			mockSetup:      func(mockDB *MockuserRepository) {},
			expectedStatus: fiber.StatusBadRequest,
			expectedBody:   `{"error":"неверный формат ID пользователя"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			app, mockDB, cleanup := setupUserTest(t)
			defer cleanup()

			tt.mockSetup(mockDB)

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
