package grpc

import (
	"context"
	"time"

	pb "gitlab.ozon.dev/gojhw1/pkg/gen/proto"
	"gitlab.ozon.dev/gojhw1/pkg/model"
	"gitlab.ozon.dev/gojhw1/pkg/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// userRepository определяет методы для работы с пользователями в репозитории
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

// UserRPCHandler реализует gRPC-сервис для управления пользователями
type UserRPCHandler struct {
	pb.UnimplementedUserRPCHandlerServer
	userRepository userRepository
}

// NewUserRPCHandler создает новый экземпляр UserRPCHandler
func NewUserRPCHandler(userRepo userRepository) *UserRPCHandler {
	return &UserRPCHandler{
		userRepository: userRepo,
	}
}

// CreateUser создает нового пользователя
func (s *UserRPCHandler) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	if req.GetUsername() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "имя пользователя не может быть пустым")
	}

	if req.GetPassword() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "пароль не может быть пустым")
	}

	role := req.GetRole()
	if role == "" {
		role = "user"
	}

	user := model.User{
		Username: req.GetUsername(),
		Role:     role,
	}

	if err := s.userRepository.Create(ctx, user, req.GetPassword()); err != nil {
		if err == repository.ErrUserAlreadyExists {
			return nil, status.Errorf(codes.AlreadyExists, "пользователь с таким именем уже существует")
		}
		return nil, status.Errorf(codes.Internal, "ошибка при создании пользователя: %v", err)
	}

	return &pb.CreateUserResponse{
		Message: "Пользователь успешно создан",
	}, nil
}

// GetUser получает информацию о пользователе по ID
func (s *UserRPCHandler) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "ID пользователя должен быть положительным числом")
	}

	user, err := s.userRepository.GetByID(ctx, req.GetId())
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, status.Errorf(codes.NotFound, "пользователь не найден")
		}
		return nil, status.Errorf(codes.Internal, "ошибка при получении пользователя: %v", err)
	}

	return convertModelsUserToProto(user), nil
}

// ListUsers получает список пользователей
func (s *UserRPCHandler) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	users, err := s.userRepository.List(ctx, req.GetSearchTerm())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ошибка при получении списка пользователей: %v", err)
	}

	protoUsers := make([]*pb.User, len(users))
	for i, user := range users {
		protoUsers[i] = convertModelsUserToProto(user)
	}

	return &pb.ListUsersResponse{
		Users: protoUsers,
		Total: int32(len(protoUsers)),
	}, nil
}

// UpdateUser обновляет информацию о пользователе
func (s *UserRPCHandler) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "ID пользователя должен быть положительным числом")
	}

	existingUser, err := s.userRepository.GetByID(ctx, req.GetId())
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, status.Errorf(codes.NotFound, "пользователь не найден")
		}
		return nil, status.Errorf(codes.Internal, "ошибка при получении пользователя: %v", err)
	}

	if req.GetUsername() != "" {
		existingUser.Username = req.GetUsername()
	}
	if req.GetRole() != "" {
		existingUser.Role = req.GetRole()
	}
	existingUser.UpdatedAt = time.Now()

	if err := s.userRepository.Update(ctx, existingUser); err != nil {
		return nil, status.Errorf(codes.Internal, "ошибка при обновлении пользователя: %v", err)
	}

	return &pb.UpdateUserResponse{
		Message: "Пользователь успешно обновлен",
	}, nil
}

// UpdatePassword обновляет пароль пользователя
func (s *UserRPCHandler) UpdatePassword(ctx context.Context, req *pb.UpdatePasswordRequest) (*pb.UpdatePasswordResponse, error) {
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "ID пользователя должен быть положительным числом")
	}

	if req.GetPassword() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "пароль не может быть пустым")
	}

	// Проверяем существование пользователя
	_, err := s.userRepository.GetByID(ctx, req.GetId())
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, status.Errorf(codes.NotFound, "пользователь не найден")
		}
		return nil, status.Errorf(codes.Internal, "ошибка при получении пользователя: %v", err)
	}

	err = s.userRepository.UpdatePassword(ctx, req.GetId(), req.GetPassword())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ошибка при обновлении пароля: %v", err)
	}

	return &pb.UpdatePasswordResponse{
		Message: "Пароль успешно обновлен",
	}, nil
}

// DeleteUser удаляет пользователя
func (s *UserRPCHandler) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "ID пользователя должен быть положительным числом")
	}

	err := s.userRepository.Delete(ctx, req.GetId())
	if err != nil {
		if err == repository.ErrUserNotFound {
			return nil, status.Errorf(codes.NotFound, "пользователь не найден")
		}
		return nil, status.Errorf(codes.Internal, "ошибка при удалении пользователя: %v", err)
	}

	return &pb.DeleteUserResponse{
		Message: "Пользователь успешно удален",
	}, nil
}
