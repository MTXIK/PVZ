package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"gitlab.ozon.dev/gojhw1/pkg/logger"
)

type auditFilterConfig struct {
	StdoutFilters []string `json:"stdout_filters"`
}

// loadFilterConfig загружает конфигурацию фильтров из JSON-файла
func loadFilterConfig(filePath string) (*auditFilterConfig, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logger.Warnf("Конфигурационный файл фильтров не найден: %s. Используются фильтры по умолчанию.", filePath)
		return &auditFilterConfig{}, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл конфигурации: %v", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл конфигурации: %v", err)
	}

	var config auditFilterConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("не удалось разобрать JSON конфигурацию: %v", err)
	}

	return &config, nil
}
