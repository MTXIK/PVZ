package grpc

import (
	"errors"
	"fmt"
	"time"

	pb "gitlab.ozon.dev/gojhw1/pkg/gen/proto"
	"gitlab.ozon.dev/gojhw1/pkg/model"
	"gitlab.ozon.dev/gojhw1/pkg/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	timeLayout      = "2006-01-02T15:04:05"
	defaultPageSize = 5
	maxPageSize     = 100
)

// parseDeadline преобразует строку в время
func parseDeadline(deadlineStr string) (time.Time, error) {
	if deadlineStr == "" {
		return time.Time{}, nil
	}

	// Попытка парсить как длительность
	if dur, err := time.ParseDuration(deadlineStr); err == nil {
		return time.Now().Add(dur), nil
	}

	// Попытка парсить как дату-время
	deadline, err := time.Parse(timeLayout, deadlineStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("неправильный формат даты, используйте YYYY-MM-DDThh:mm:ss или длительность")
	}
	return deadline, nil
}

// ConvertModelOrderToProto преобразует модель заказа в protobuf формат
func convertModelOrderToProto(order model.Order) *pb.Order {
	protoOrder := &pb.Order{
		Id:         order.ID,
		CustomerId: order.CustomerID,
		Weight:     order.Weight,
		Cost:       order.Cost,
		UpdatedAt:  timestamppb.New(order.UpdatedAt),
	}

	// Установка состояния заказа
	switch order.State {
	case model.StateAccepted:
		protoOrder.State = pb.OrderState_ORDER_STATE_ACCEPTED
	case model.StateDelivered:
		protoOrder.State = pb.OrderState_ORDER_STATE_DELIVERED
	case model.StateReturned:
		protoOrder.State = pb.OrderState_ORDER_STATE_RETURNED
	}

	// Дедлайн
	if !order.DeadlineAt.IsZero() {
		protoOrder.DeadlineAt = timestamppb.New(order.DeadlineAt)
	}

	// Время доставки
	if order.DeliveredAt != nil && !order.DeliveredAt.IsZero() {
		protoOrder.DeliveredAt = timestamppb.New(*order.DeliveredAt)
	}

	// Время возврата
	if order.ReturnedAt != nil && !order.ReturnedAt.IsZero() {
		protoOrder.ReturnedAt = timestamppb.New(*order.ReturnedAt)
	}

	// Тип упаковки
	if order.PackageType != nil {
		switch *order.PackageType {
		case model.PackageBag:
			protoOrder.PackageType = pb.PackageType_PACKAGE_TYPE_BAG
		case model.PackageBox:
			protoOrder.PackageType = pb.PackageType_PACKAGE_TYPE_BOX
		case model.PackageFilm:
			protoOrder.PackageType = pb.PackageType_PACKAGE_TYPE_FILM
		}
	}

	// Тип обертки
	if order.Wrapper != nil {
		switch *order.Wrapper {
		case model.WrapperFilm:
			protoOrder.Wrapper = pb.WrapperType_WRAPPER_TYPE_FILM
		}
	}

	return protoOrder
}

// ConvertModelsUserToProto преобразует модель пользователя в protobuf формат
func convertModelsUserToProto(user model.User) *pb.User {
	return &pb.User{
		Id:        user.ID,
		Username:  user.Username,
		Role:      user.Role,
		CreatedAt: timestamppb.New(user.CreatedAt),
		UpdatedAt: timestamppb.New(user.UpdatedAt),
	}
}

// PackageTypeFromProto преобразует protobuf тип упаковки в модель
func packageTypeFromProto(packageType pb.PackageType) *model.PackageType {
	var pt model.PackageType

	switch packageType {
	case pb.PackageType_PACKAGE_TYPE_BAG:
		pt = model.PackageBag
		return &pt
	case pb.PackageType_PACKAGE_TYPE_BOX:
		pt = model.PackageBox
		return &pt
	case pb.PackageType_PACKAGE_TYPE_FILM:
		pt = model.PackageFilm
		return &pt
	default:
		return nil
	}
}

// WrapperTypeFromProto преобразует protobuf тип обертки в модель
func wrapperTypeFromProto(wrapperType pb.WrapperType) *model.WrapperType {
	var wt model.WrapperType

	switch wrapperType {
	case pb.WrapperType_WRAPPER_TYPE_FILM:
		wt = model.WrapperFilm
		return &wt
	default:
		return nil
	}
}

// ParseGRPCError преобразует ошибки в gRPC статус-коды
func parseGRPCError(err error) error {
	switch {
	case err == nil:
		return nil

	// Bad Request errors
	case errors.Is(err, service.ErrStorageDeadlinePassed),
		errors.Is(err, service.ErrDeadlineNotExpired),
		errors.Is(err, service.ErrNotDelivered),
		errors.Is(err, service.ErrOpenFile),
		errors.Is(err, service.ErrReadFile),
		errors.Is(err, service.ErrParseFile),
		errors.Is(err, service.ErrInvalidDateFormat),
		errors.Is(err, service.ErrNegativeWeight),
		errors.Is(err, service.ErrInvalidOrderID),
		errors.Is(err, service.ErrPackageWeightExceeded),
		errors.Is(err, service.ErrUnknownPackageType),
		errors.Is(err, service.ErrUnknownWrapperType),
		errors.Is(err, service.ErrNegativeCost):
		return status.Errorf(codes.InvalidArgument, err.Error())

	// Conflict errors
	case errors.Is(err, service.ErrOrderExists),
		errors.Is(err, service.ErrOrderAlreadyDelivered),
		errors.Is(err, service.ErrWrongState):
		return status.Errorf(codes.AlreadyExists, err.Error())

	// Forbidden errors
	case errors.Is(err, service.ErrWrongCustomer):
		return status.Errorf(codes.PermissionDenied, err.Error())

	// Not Found errors
	case errors.Is(err, errors.New("пользователь не найден")),
		errors.Is(err, errors.New("заказ не найден")):
		return status.Errorf(codes.NotFound, err.Error())

	// Default case for unhandled errors
	default:
		return status.Errorf(codes.Internal, err.Error())
	}
}
