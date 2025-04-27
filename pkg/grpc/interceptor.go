package grpc

import (
	"context"
	"encoding/base64"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type ctxKey string

const (
	usernameKey ctxKey = "username"
)

type BasicAuthInterceptor struct {
	userRepository userRepository
}

func NewBasicAuthInterceptor(userRepository userRepository) *BasicAuthInterceptor {
	return &BasicAuthInterceptor{
		userRepository: userRepository,
	}
}

// UnaryInterceptor обрабатывает унарные RPC-вызовы
func (i *BasicAuthInterceptor) UnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	// Пропускаем аутентификацию для метода CreateUser, аналогично register в HTTP версии
	if info.FullMethod == "/proto.UserRPCHandler/CreateUser" {
		return handler(ctx, req)
	}

	// Получаем метаданные из контекста
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "метаданные не найдены")
	}

	// Ищем заголовок авторизации
	authHeader, ok := md["authorization"]
	if !ok || len(authHeader) == 0 {
		return nil, status.Errorf(codes.Unauthenticated, "требуется авторизация")
	}

	// Извлекаем и проверяем учетные данные
	username, password, ok := parseBasicAuth(authHeader[0])
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "некорректный формат заголовка авторизации")
	}

	// Проверяем учетные данные
	if !i.userRepository.CheckPassword(ctx, username, password) {
		return nil, status.Errorf(codes.Unauthenticated, "неверные учетные данные")
	}

	// Добавляем имя пользователя в контекст (по аналогии с ContextUsername в fiber)
	newCtx := context.WithValue(ctx, usernameKey, username)

	// Вызываем обработчик с обновленным контекстом
	return handler(newCtx, req)
}

// StreamInterceptor обрабатывает потоковые RPC-вызовы
func (i *BasicAuthInterceptor) StreamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	// Получаем метаданные из контекста
	ctx := ss.Context()
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "метаданные не найдены")
	}

	// Ищем заголовок авторизации
	authHeader, ok := md["authorization"]
	if !ok || len(authHeader) == 0 {
		return status.Errorf(codes.Unauthenticated, "требуется авторизация")
	}

	// Извлекаем и проверяем учетные данные
	username, password, ok := parseBasicAuth(authHeader[0])
	if !ok {
		return status.Errorf(codes.Unauthenticated, "некорректный формат заголовка авторизации")
	}

	// Проверяем учетные данные
	if !i.userRepository.CheckPassword(ctx, username, password) {
		return status.Errorf(codes.Unauthenticated, "неверные учетные данные")
	}

	// Создаем обертку для ServerStream с добавленным username в контекст
	wrappedStream := NewWrappedServerStream(ss, context.WithValue(ctx, usernameKey, username))

	// Вызываем обработчик с оберткой потока
	return handler(srv, wrappedStream)
}

// parseBasicAuth извлекает учетные данные из заголовка Basic Auth
func parseBasicAuth(auth string) (username, password string, ok bool) {
	// "Basic dXNlcm5hbWU6cGFzc3dvcmQ="
	const prefix = "Basic "
	if !strings.HasPrefix(auth, prefix) {
		return "", "", false
	}

	// Декодируем credentials из base64
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return "", "", false
	}

	// Разделяем на username:password
	cs := string(c)
	parts := strings.Split(cs, ":")
	if len(parts) != 2 {
		return "", "", false
	}

	return parts[0], parts[1], true
}

// WrappedServerStream оборачивает grpc.ServerStream для изменения контекста
type WrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// NewWrappedServerStream создает новую обертку для ServerStream
func NewWrappedServerStream(stream grpc.ServerStream, ctx context.Context) grpc.ServerStream {
	return &WrappedServerStream{
		ServerStream: stream,
		ctx:          ctx,
	}
}

// Context возвращает модифицированный контекст
func (w *WrappedServerStream) Context() context.Context {
	return w.ctx
}
