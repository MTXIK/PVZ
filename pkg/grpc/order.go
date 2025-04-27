package grpc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	pb "gitlab.ozon.dev/gojhw1/pkg/gen/proto"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// OrderRPCHandler реализует gRPC сервис для работы с заказами
type OrderRPCHandler struct {
	pb.UnimplementedOrderRPCHandlerServer
	orderRPCHandler orderServiceInterface
}

// orderServiceInterface описывает интерфейс сервиса для работы с заказами
type orderServiceInterface interface {
	AcceptOrder(ctx context.Context, id, customerID int64, deadline time.Time, weight, cost float64, packageType *model.PackageType, wrapper *model.WrapperType) error
	ReturnOrderToCourier(ctx context.Context, id int64) error
	DeliverOrder(ctx context.Context, id, customerID int64, now time.Time) error
	ProcessReturnOrder(ctx context.Context, id, customerID int64, now time.Time) error
	OrderHistory(ctx context.Context, searchTerm string) ([]model.Order, error)
	AcceptOrdersFromFile(ctx context.Context, filename string) error
	GetOrderByID(ctx context.Context, id int64) (model.Order, error)
	ClearDatabase(ctx context.Context) error
	ListOrdersWithCursor(ctx context.Context, cursorID int64, limit int, customerID int64, filterPVZ bool, searchTerm string) ([]model.Order, error)
	ListReturnsWithCursor(ctx context.Context, cursorID int64, limit int, searchTerm string) ([]model.Order, error)
}

// NewOrderRPCHandler создает новый экземпляр OrderRPCHandler
func NewOrderRPCHandler(orderRPCHandler orderServiceInterface) *OrderRPCHandler {
	return &OrderRPCHandler{
		orderRPCHandler: orderRPCHandler,
	}
}

// CreateOrder создает новый заказ
func (s *OrderRPCHandler) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.Order, error) {
	if req.GetWeight() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "вес должен быть больше 0")
	}

	if req.GetCost() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "стоимость должна быть больше 0")
	}

	deadline, err := parseDeadline(req.GetDeadlineAt())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}

	packageType := packageTypeFromProto(req.GetPackageType())
	wrapper := wrapperTypeFromProto(req.GetWrapper())

	err = s.orderRPCHandler.AcceptOrder(
		ctx,
		req.GetId(),
		req.GetCustomerId(),
		deadline,
		req.GetWeight(),
		req.GetCost(),
		packageType,
		wrapper,
	)

	if err != nil {
		return nil, parseGRPCError(err)
	}

	order, err := s.orderRPCHandler.GetOrderByID(ctx, req.GetId())
	if err != nil {
		return nil, parseGRPCError(err)
	}

	return convertModelOrderToProto(order), nil
}

// GetOrder получает информацию о заказе по ID
func (s *OrderRPCHandler) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.Order, error) {
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "ID заказа должен быть положительным числом")
	}

	order, err := s.orderRPCHandler.GetOrderByID(ctx, req.GetId())
	if err != nil {
		return nil, parseGRPCError(err)
	}

	return convertModelOrderToProto(order), nil
}

// ReturnToCourier обрабатывает возврат заказа курьеру
func (s *OrderRPCHandler) ReturnToCourier(ctx context.Context, req *pb.ReturnToCourierRequest) (*pb.ReturnToCourierResponse, error) {
	if req.GetId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "ID заказа должен быть положительным числом")
	}

	err := s.orderRPCHandler.ReturnOrderToCourier(ctx, req.GetId())
	if err != nil {
		return nil, parseGRPCError(err)
	}

	return &pb.ReturnToCourierResponse{
		Message: fmt.Sprintf("Заказ %d возвращен курьеру", req.GetId()),
	}, nil
}

// ProcessCustomer обрабатывает действия с заказами для указанного клиента
func (s *OrderRPCHandler) ProcessCustomer(ctx context.Context, req *pb.ProcessCustomerRequest) (*pb.ProcessCustomerResponse, error) {
	if req.GetCustomerId() <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "ID клиента должен быть положительным числом")
	}

	if req.GetAction() != "handout" && req.GetAction() != "return" {
		return nil, status.Errorf(codes.InvalidArgument, "неизвестное действие")
	}

	now := time.Now()
	results := make([]*pb.ProcessingResult, 0, len(req.GetOrderIds()))

	for _, orderID := range req.GetOrderIds() {
		var err error

		switch req.GetAction() {
		case "handout":
			err = s.orderRPCHandler.DeliverOrder(ctx, orderID, req.GetCustomerId(), now)
		case "return":
			err = s.orderRPCHandler.ProcessReturnOrder(ctx, orderID, req.GetCustomerId(), now)
		}

		result := &pb.ProcessingResult{
			OrderId: orderID,
			Status:  int32(codes.OK),
		}

		if err != nil {
			grpcErr, ok := status.FromError(parseGRPCError(err))
			if ok {
				result.Status = int32(grpcErr.Code())
				result.Result = &pb.ProcessingResult_Error{Error: grpcErr.Message()}
			} else {
				result.Status = int32(codes.Internal)
				result.Result = &pb.ProcessingResult_Error{Error: err.Error()}
			}
		} else {
			result.Result = &pb.ProcessingResult_Message{
				Message: fmt.Sprintf("Заказ ID %d успешно %s клиенту %d", orderID, req.GetAction(), req.GetCustomerId()),
			}
		}

		results = append(results, result)
	}

	return &pb.ProcessCustomerResponse{
		Results: results,
	}, nil
}

// ListOrders получает список заказов с курсорной пагинацией
func (s *OrderRPCHandler) ListOrders(ctx context.Context, req *pb.ListOrdersRequest) (*pb.ListOrdersResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	orders, err := s.orderRPCHandler.ListOrdersWithCursor(
		ctx,
		req.GetCursorId(),
		limit+1,
		req.GetCustomerId(),
		req.GetFilterPvz(),
		req.GetSearchTerm(),
	)
	if err != nil {
		return nil, parseGRPCError(err)
	}

	hasMore := len(orders) > limit
	var nextCursor int64

	if hasMore {
		orders = orders[:limit]
	}

	if len(orders) > 0 {
		nextCursor = orders[len(orders)-1].ID
	}

	protoOrders := make([]*pb.Order, len(orders))
	for i, order := range orders {
		protoOrders[i] = convertModelOrderToProto(order)
	}

	return &pb.ListOrdersResponse{
		Orders:     protoOrders,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, nil
}

// ListReturns получает список возвращенных заказов с курсорной пагинацией
func (s *OrderRPCHandler) ListReturns(ctx context.Context, req *pb.ListReturnsRequest) (*pb.ListReturnsResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}

	returns, err := s.orderRPCHandler.ListReturnsWithCursor(
		ctx,
		req.GetCursorId(),
		limit+1,
		req.GetSearchTerm(),
	)
	if err != nil {
		return nil, parseGRPCError(err)
	}

	hasMore := len(returns) > limit
	var nextCursor int64

	if hasMore {
		returns = returns[:limit]
	}

	if len(returns) > 0 {
		nextCursor = returns[len(returns)-1].ID
	}

	protoReturns := make([]*pb.Order, len(returns))
	for i, ret := range returns {
		protoReturns[i] = convertModelOrderToProto(ret)
	}

	return &pb.ListReturnsResponse{
		Returns:    protoReturns,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, nil
}

// OrderHistory получает историю всех заказов
func (s *OrderRPCHandler) OrderHistory(ctx context.Context, req *pb.OrderHistoryRequest) (*pb.OrderHistoryResponse, error) {
	orders, err := s.orderRPCHandler.OrderHistory(ctx, req.GetSearchTerm())
	if err != nil {
		return nil, parseGRPCError(err)
	}

	protoOrders := make([]*pb.Order, len(orders))

	for i, order := range orders {
		protoOrders[i] = convertModelOrderToProto(order)
	}

	return &pb.OrderHistoryResponse{
		Orders: protoOrders,
		Total:  int32(len(protoOrders)),
	}, nil
}

// AcceptOrdersFromFile загружает заказы из файла
func (s *OrderRPCHandler) AcceptOrdersFromFile(ctx context.Context, req *pb.AcceptOrdersFromFileRequest) (*pb.AcceptOrdersFromFileResponse, error) {
	if len(req.GetFileContent()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "файл не может быть пустым")
	}

	tempDir := "temp"
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		if err := os.Mkdir(tempDir, 0755); err != nil {
			return nil, status.Errorf(codes.Internal, "ошибка при создании временной директории: %v", err)
		}
	}

	filename := req.GetFilename()
	if filename == "" {
		filename = fmt.Sprintf("order_upload_%d.json", time.Now().Unix())
	}

	tempFilePath := filepath.Join(tempDir, filename)

	if err := os.WriteFile(tempFilePath, req.GetFileContent(), 0644); err != nil {
		return nil, status.Errorf(codes.Internal, "ошибка при записи во временный файл: %v", err)
	}

	defer func() {
		if removeErr := os.Remove(tempFilePath); removeErr != nil {
			logger.Warnf("Ошибка при удалении временного файла %s: %v\n", tempFilePath, removeErr)
		}

		if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
			if files, err := os.ReadDir(tempDir); err == nil && len(files) == 0 {
				if removeErr := os.Remove(tempDir); removeErr != nil {
					logger.Warnf("Ошибка при удалении временной директории %s: %v\n", tempDir, removeErr)
				}
			}
		}
	}()

	if err := s.orderRPCHandler.AcceptOrdersFromFile(ctx, tempFilePath); err != nil {
		return nil, parseGRPCError(err)
	}

	return &pb.AcceptOrdersFromFileResponse{
		Message: "Заказы успешно загружены из файла",
	}, nil
}

// ClearDatabase очищает базу данных
func (s *OrderRPCHandler) ClearDatabase(ctx context.Context, _ *emptypb.Empty) (*pb.ClearDatabaseResponse, error) {
	if err := s.orderRPCHandler.ClearDatabase(ctx); err != nil {
		return nil, parseGRPCError(err)
	}

	return &pb.ClearDatabaseResponse{
		Message: "База данных успешно очищена",
	}, nil
}
