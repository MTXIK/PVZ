package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

// auditRepository интерфейс для работы с аудит-логами в репозитории
type auditRepository interface {
	FetchTasksIDs(ctx context.Context, limit int) ([]model.AuditIDs, error)
	GetAuditLog(ctx context.Context, id uint64) (model.AuditLog, error)
	MarkTaskFailed(ctx context.Context, taskID uint64, taskErr error) error
	MarkTaskCompleted(ctx context.Context, taskID uint64) error
}

// producerInterface интерфейс для отправки сообщений в Kafka
type producerInterface interface {
	SendMessage(ctx context.Context, logID uint64, payload model.AuditLog) error
	Close() error
}

// OutboxProducer реализует продюсера для отправки сообщений в Kafka
type OutboxProducer struct {
	producer sarama.SyncProducer
	topic    string
}

// OutboxWorkerPool реализует пул воркеров для обработки outbox сообщений
type OutboxWorkerPool struct {
	workersNum int
	batchSize  int
	polingRate time.Duration
	auditRepo  auditRepository
	producer   producerInterface
	wg         sync.WaitGroup
	cancel     context.CancelFunc
}

// NewOutboxProducer создает новый экземпляр OutboxProducer
func NewOutboxProducer(brokers []string, topic string) (*OutboxProducer, error) {
	config := sarama.NewConfig()
	config.Producer.RequiredAcks = sarama.WaitForAll
	config.Producer.Retry.Max = 3
	config.Producer.Return.Successes = true

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, err
	}

	return &OutboxProducer{
		producer: producer,
		topic:    topic,
	}, nil
}

// SendMessage отправляет сообщение в Kafka
func (p *OutboxProducer) SendMessage(ctx context.Context, taskID uint64, payload model.AuditLog) error {
	data, err := json.Marshal(payload)
	if err != nil {
		logger.Errorf("Ошибка маршалинга данных для Kafka (taskID=%d): %v", taskID, err)
		return fmt.Errorf("ошибка маршалинга данных для Kafka: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: p.topic,
		Key:   sarama.StringEncoder(fmt.Sprintf("%d", taskID)),
		Value: sarama.ByteEncoder(data),
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		logger.Errorf("Ошибка отправки сообщения в Kafka (taskID=%d): %v", taskID, err)
		return fmt.Errorf("ошибка отправки сообщения в Kafka: %w", err)
	}

	logger.Debugf("Сообщение успешно отправлено в Kafka: topic=%s, partition=%d, offset=%d, taskID=%d",
		p.topic, partition, offset, taskID)

	return nil
}

// Close закрывает соединение с Kafka
func (p *OutboxProducer) Close() error {
	logger.Info("Закрытие соединения с Kafka продюсером")
	return p.producer.Close()
}

// NewOutboxWorkerPool создает новый пул воркеров для обработки outbox сообщений
func NewOutboxWorkerPool(
	auditRepo auditRepository,
	producer producerInterface,
	workersNum, batchSize int,
	pollingRate time.Duration,
) *OutboxWorkerPool {

	logger.Infof("Создание Outbox Worker Pool: workersNum=%d, batchSize=%d, pollingRate=%v",
		workersNum, batchSize, pollingRate)

	pool := &OutboxWorkerPool{
		workersNum: workersNum,
		batchSize:  batchSize,
		polingRate: pollingRate,
		auditRepo:  auditRepo,
		producer:   producer,
	}

	return pool
}

// Start запускает пул воркеров для обработки outbox сообщений
func (p *OutboxWorkerPool) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	logger.Infof("Запускаем Outbox Worker Pool с %d воркерами", p.workersNum)

	p.wg.Add(p.workersNum)
	for i := range p.workersNum {
		go p.workerRoutine(ctx, i+1)
	}
}

// Stop останавливает работу пула воркеров
func (p *OutboxWorkerPool) Stop() {
	logger.Info("Останавливаем Outbox Worker Pool")
	p.cancel()
	p.wg.Wait()
	logger.Info("Outbox Worker Pool завершил работу")
}

// workerRoutine выполняет основной цикл работы воркера
func (p *OutboxWorkerPool) workerRoutine(ctx context.Context, workerID int) {
	defer p.wg.Done()

	workerName := fmt.Sprintf("OutboxWorker-%d", workerID)
	logger.Infof("[%s] Воркер запущен", workerName)

	ticker := time.NewTicker(p.polingRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			select {
			case <-ctx.Done():
				logger.Infof("[%s] Воркер завершает работу", workerName)
				return
			default:
				p.processOutboxBatch(ctx, workerName)
			}
		case <-ctx.Done():
			logger.Infof("[%s] Воркер завершает работу", workerName)
			return
		}
	}
}

// processOutboxBatch обрабатывает пакет задач из очереди outbox
func (p *OutboxWorkerPool) processOutboxBatch(ctx context.Context, workerName string) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	auditIDs, err := p.auditRepo.FetchTasksIDs(ctx, p.batchSize)
	if err != nil {
		logger.Errorf("[%s] Ошибка при получении ID задач: %v", workerName, err)
		return
	}

	if len(auditIDs) == 0 {
		logger.Debugf("[%s] Нет новых задач для обработки", workerName)
		return
	}

	logger.Infof("[%s] Получено %d задач для обработки", workerName, len(auditIDs))

	for _, auditID := range auditIDs {
		logger.Debugf("[%s] Обработка задачи: taskID=%d, logID=%d", workerName, auditID.TaskID, auditID.LogID)
		auditLog, err := p.auditRepo.GetAuditLog(ctx, auditID.LogID)
		if err != nil {
			logger.Errorf("[%s] Ошибка при получении аудит лога (ID=%d): %v", workerName, auditID.LogID, err)
			markErr := p.auditRepo.MarkTaskFailed(ctx, auditID.TaskID, err)
			if markErr != nil {
				logger.Errorf("[%s] Ошибка при маркировке задачи %d как проваленной: %v", workerName, auditID.TaskID, markErr)
			}
			continue
		}

		err = p.producer.SendMessage(ctx, auditID.TaskID, auditLog)
		if err != nil {
			logger.Errorf("[%s] Ошибка при отправке сообщения в Kafka (taskID=%d): %v", workerName, auditID.TaskID, err)
			markErr := p.auditRepo.MarkTaskFailed(ctx, auditID.TaskID, err)
			if markErr != nil {
				logger.Errorf("[%s] Ошибка при маркировке задачи %d как проваленной: %v", workerName, auditID.TaskID, markErr)
			}
			continue
		}

		err = p.auditRepo.MarkTaskCompleted(ctx, auditID.TaskID)
		if err != nil {
			logger.Errorf("[%s] Ошибка маркировки задачи %d как выполненной: %v", workerName, auditID.TaskID, err)
		} else {
			logger.Debugf("[%s] Задача %d успешно обработана", workerName, auditID.TaskID)
		}
	}
}
