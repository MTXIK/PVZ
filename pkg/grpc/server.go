package grpc

import (
	"fmt"
	"net"

	pb "gitlab.ozon.dev/gojhw1/pkg/gen/proto"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Server представляет gRPC сервер
type Server struct {
	grpcServer   *grpc.Server
	host         string
	port         string
	userService  *UserRPCHandler
	orderService *OrderRPCHandler
}

// NewServer создает новый экземпляр gRPC сервера
func NewServer(host, port string, userRepo userRepository, orderService orderServiceInterface) *Server {
	authInterceptor := NewBasicAuthInterceptor(userRepo)

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			authInterceptor.UnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			authInterceptor.StreamInterceptor,
		),
	)

	userService := NewUserRPCHandler(userRepo)
	orderRpcService := NewOrderRPCHandler(orderService)

	pb.RegisterUserRPCHandlerServer(grpcServer, userService)
	pb.RegisterOrderRPCHandlerServer(grpcServer, orderRpcService)

	reflection.Register(grpcServer)

	return &Server{
		grpcServer:   grpcServer,
		host:         host,
		port:         port,
		userService:  userService,
		orderService: orderRpcService,
	}
}

// Start запускает gRPC сервер
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%s", s.host, s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("не удалось запустить сервер на порту %s: %w", s.port, err)
	}

	fmt.Printf("gRPC сервер запущен на порту %s\n", s.port)
	return s.grpcServer.Serve(listener)
}

// Stop останавливает gRPC сервер
func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
}
