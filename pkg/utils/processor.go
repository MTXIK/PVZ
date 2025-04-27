package utils

import (
	"context"
	"encoding/json"
	"strings"

	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

const configPath = "./pkg/utils/audit_filters.json"

type auditRepository interface {
	CreateLogsWithTasks(ctx context.Context, logs []model.AuditLog) error
}

type logProcessor interface {
	processLogs(ctx context.Context, workerName string, batch []model.AuditLog) error
	name() string
}

type stdoutLogProcessor struct {
	filters []string
}

func newStdoutLogProcessor() *stdoutLogProcessor {
	config, err := loadFilterConfig(configPath)
	if err != nil {
		logger.Warnf("Ошибка загрузки конфигурации фильтров: %v. Фильтры не будут применены.", err)
		return &stdoutLogProcessor{}
	}

	if len(config.StdoutFilters) > 0 {
		logger.Infof("Загружены фильтры для stdout: %v", config.StdoutFilters)
	} else {
		logger.Infof("Фильтры для stdout не заданы. Будут выводиться все логи.")
	}

	return &stdoutLogProcessor{
		filters: config.StdoutFilters,
	}
}

func (p *stdoutLogProcessor) name() string {
	return "stdout"
}

func (p *stdoutLogProcessor) processLogs(ctx context.Context, workerName string, batch []model.AuditLog) error {
	for _, myLog := range batch {
		if len(p.filters) == 0 {
			p.printLog(workerName, myLog)
			continue
		}

		data, err := json.Marshal(myLog)
		if err != nil {
			logger.Errorf("[%s] Ошибка маршалинга лога для фильтрации: %v", workerName, err)
			continue
		}

		logStr := strings.ToLower(string(data))

		for _, filter := range p.filters {
			if strings.Contains(logStr, strings.ToLower(filter)) {
				p.printLog(workerName, myLog)
				break
			}
		}
	}

	return nil
}

// printLog вспомогательный метод для форматирования и вывода лога
func (p *stdoutLogProcessor) printLog(workerName string, myLog model.AuditLog) {
	data, err := json.MarshalIndent(myLog, "", "  ")
	if err != nil {
		logger.Errorf("[%s] Ошибка маршалинга лога: %v", workerName, err)
		return
	}

	logger.Infof("[AUDIT] %s", string(data))
}

type dbLogProcessor struct {
	repo auditRepository
}

func newDBLogProcessor(repo auditRepository) *dbLogProcessor {
	return &dbLogProcessor{repo: repo}
}

func (p *dbLogProcessor) name() string {
	return "db"
}

func (p *dbLogProcessor) processLogs(ctx context.Context, workerName string, batch []model.AuditLog) error {
	if err := p.repo.CreateLogsWithTasks(ctx, batch); err != nil {
		logger.Errorf("[%s] Ошибка записи логов в БД: %v", workerName, err)
		return err
	}

	return nil
}
