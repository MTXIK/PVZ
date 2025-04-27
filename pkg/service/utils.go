package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"gitlab.ozon.dev/gojhw1/pkg/model"
)

type orderFileData struct {
	ID          int64   `json:"id"`
	CustomerID  int64   `json:"customer_id"`
	DeadlineAt  string  `json:"deadline_at"`
	Weight      float64 `json:"weight"`
	Cost        float64 `json:"cost"`
	PackageType string  `json:"package_type,omitempty"`
	Wrapper     string  `json:"wrapper,omitempty"`
}

// readOrdersFromFile читает и парсит JSON файл с заказами
func readOrdersFromFile(filename string) ([]orderFileData, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOpenFile, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReadFile, err)
	}

	var orders []orderFileData
	if err = json.Unmarshal(data, &orders); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseFile, err)
	}

	return orders, nil
}

// parseDeadline парсит дедлайн из строки
func parseDeadline(deadlineStr string) (time.Time, error) {
	if dur, err := time.ParseDuration(deadlineStr); err == nil {
		return time.Now().Add(dur), nil
	}

	deadline, err := time.Parse(timeLayout, deadlineStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %w", ErrInvalidDateFormat, err)
	}

	return deadline, nil
}

// processPackaging обрабатывает параметры упаковки
func processPackaging(packageStr, wrapperStr string) (*model.PackageType, *model.WrapperType) {
	var packageType *model.PackageType
	var wrapper *model.WrapperType

	if packageStr != "" {
		pt := model.PackageType(packageStr)
		packageType = &pt
	}

	if wrapperStr != "" {
		wt := model.WrapperType(wrapperStr)
		wrapper = &wt
	}

	return packageType, wrapper
}
