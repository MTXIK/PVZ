package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"gitlab.ozon.dev/gojhw1/pkg/model"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
)

type userRepository interface {
	Create(ctx context.Context, user model.User, plainPassword string) error
	Update(ctx context.Context, user model.User) error
	UpdatePassword(ctx context.Context, userID int64, newPassword string) error
	Delete(ctx context.Context, id int64) error
	GetByID(ctx context.Context, id int64) (model.User, error)
	GetByUsername(ctx context.Context, username string) (model.User, error)
	List(ctx context.Context, searchTerm string) ([]model.User, error)
	CheckPassword(ctx context.Context, username, password string) bool
}

// createUserRequest представляет собой структуру запроса для создания нового пользователя
type сreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// UserHandler обработчик запросов для управления пользователями
type UserHandler struct {
	userRepository userRepository
}

// NewUserHandler создает новый обработчик пользователей
func NewUserHandler(userRepo userRepository) *UserHandler {
	return &UserHandler{
		userRepository: userRepo,
	}
}

// CreateUser обрабатывает запрос на создание нового пользователя
func (h *UserHandler) CreateUser(c *fiber.Ctx) error {
	ctx := c.UserContext()

	var req сreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Ошибка при разборе запроса",
		})
	}

	// Валидация входных данных
	if err := validateCreateUserRequest(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Если роль не указана, используем роль по умолчанию
	if req.Role == "" {
		req.Role = "user"
	}

	user := model.User{
		Username: req.Username,
		Role:     req.Role,
	}

	if err := h.userRepository.Create(ctx, user, req.Password); err != nil {
		if err.Error() == repository.ErrUserAlreadyExists.Error() {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Пользователь с таким именем уже существует",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка при создании пользователя",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Пользователь успешно создан",
	})
}

// GetUser обрабатывает запрос на получение информации о пользователе
func (h *UserHandler) GetUser(c *fiber.Ctx) error {
	ctx := c.UserContext()

	id, err := parseUserIDFromParams(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	user, err := h.userRepository.GetByID(ctx, id)
	if err != nil {
		if err.Error() == repository.ErrUserNotFound.Error() {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Пользователь не найден",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка при получении пользователя",
		})
	}

	return c.Status(fiber.StatusOK).JSON(user)
}

// ListUsers обрабатывает запрос на получение списка пользователей
func (h *UserHandler) ListUsers(c *fiber.Ctx) error {
	ctx := c.UserContext()
	searchTerm := c.Query("search", "")

	users, err := h.userRepository.List(ctx, searchTerm)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка при получении списка пользователей",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"users": users,
		"total": len(users),
	})
}

// UpdateUser обрабатывает запрос на обновление информации о пользователе
func (h *UserHandler) UpdateUser(c *fiber.Ctx) error {
	ctx := c.UserContext()

	id, err := parseUserIDFromParams(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	type UpdateUserRequest struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}

	var req UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Ошибка при разборе запроса",
		})
	}

	existingUser, err := h.userRepository.GetByID(ctx, id)
	if err != nil {
		if err.Error() == repository.ErrUserNotFound.Error() {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Пользователь не найден",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка при получении пользователя",
		})
	}

	if req.Username != "" {
		existingUser.Username = req.Username
	}
	if req.Role != "" {
		existingUser.Role = req.Role
	}
	existingUser.UpdatedAt = time.Now()

	if err := h.userRepository.Update(ctx, existingUser); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка при обновлении пользователя",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Пользователь успешно обновлен",
	})
}

// UpdatePassword обрабатывает запрос на обновление пароля пользователя
func (h *UserHandler) UpdatePassword(c *fiber.Ctx) error {
	ctx := c.UserContext()

	id, err := parseUserIDFromParams(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	type UpdatePasswordRequest struct {
		Password string `json:"password"`
	}

	var req UpdatePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Ошибка при разборе запроса",
		})
	}

	if err := validatePasswordRequest(req.Password); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	_, err = h.userRepository.GetByID(ctx, id)
	if err != nil {
		if err.Error() == repository.ErrUserNotFound.Error() {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Пользователь не найден",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка при получении пользователя",
		})
	}

	err = h.userRepository.UpdatePassword(ctx, id, req.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка при обновлении пароля",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Пароль успешно обновлен",
	})
}

// DeleteUser обрабатывает запрос на удаление пользователя
func (h *UserHandler) DeleteUser(c *fiber.Ctx) error {
	ctx := c.UserContext()

	id, err := parseUserIDFromParams(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	err = h.userRepository.Delete(ctx, id)
	if err != nil {
		if err.Error() == repository.ErrUserNotFound.Error() {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Пользователь не найден",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Ошибка при удалении пользователя",
		})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Пользователь успешно удален",
	})
}
