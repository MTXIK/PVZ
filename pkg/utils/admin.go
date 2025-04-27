package utils

import (
	"context"

	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

type UserAdminIniter interface {
	Create(ctx context.Context, user model.User, plainPassword string) error
	List(ctx context.Context, searchTerm string) ([]model.User, error)
}

// InitDefaultUser создает пользователя админа, если в базе нет пользователей
func InitDefaultUser(repo UserAdminIniter) {
	ctx := context.Background()

	users, err := repo.List(ctx, "")
	if err != nil {
		logger.Errorf("Ошибка при получении списка пользователей: %v", err)
		return
	}

	if len(users) == 0 {
		logger.Info("Создаем пользователя админа по умолчанию")
		user := model.User{
			Username: "admin",
			Role:     "admin",
		}

		password := "admin"

		err := repo.Create(ctx, user, password)
		if err != nil {
			logger.Errorf("Ошибка при создании пользователя админа: %v", err)
			return
		}

		logger.Info("Пользователь админ успешно создан. Логин: admin, пароль: admin")
	}
}
