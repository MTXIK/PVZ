package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

// Consumer представляет консьюмера Kafka
type Consumer struct {
	consumer sarama.ConsumerGroup
	topics   []string
	wg       sync.WaitGroup
	cancel   context.CancelFunc
}

// ConsumerHandler реализует интерфейс sarama.ConsumerGroupHandler
type ConsumerHandler struct {
	ready chan struct{}
}

// NewConsumer создает новый экземпляр Consumer для Kafka
func NewConsumer(brokers []string, groupID string, topics []string) (*Consumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Rebalance.Strategy = sarama.NewBalanceStrategyRoundRobin()
	config.Consumer.Return.Errors = true
	config.Consumer.Offsets.Retry.Max = 3

	logger.Infof("Создание консьюмера Kafka: brokers=%v, groupID=%s, topics=%v", brokers, groupID, topics)

	consumer, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		logger.Errorf("Ошибка создания consumer group: %v", err)
		return nil, fmt.Errorf("ошибка создания consumer group: %w", err)
	}

	return &Consumer{
		consumer: consumer,
		topics:   topics,
	}, nil
}

// Start запускает процесс потребления сообщений из Kafka
func (c *Consumer) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	logger.Info("Запуск Kafka консьюмера")

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		handlerReady := make(chan struct{})

		// Начальные значения для экспоненциальной задержки
		baseDelay := 100 * time.Millisecond
		maxDelay := 30 * time.Second
		consecutiveErrors := 0

		for {
			handler := &ConsumerHandler{ready: handlerReady}

			select {
			case <-ctx.Done():
				logger.Info("Консьюмер остановлен из-за завершения контекста")
				return
			default:
				logger.Debug("Запуск обработчика Consume для чтения сообщений")
				if err := c.consumer.Consume(ctx, c.topics, handler); err != nil {
					consecutiveErrors++
					// Вычисляем задержку с экспоненциальным ростом
					backoff := time.Duration(math.Min(
						float64(maxDelay),
						float64(baseDelay)*math.Pow(2, float64(consecutiveErrors-1)),
					))

					logger.Errorf("Ошибка при потреблении сообщений (попытка %d): %v. Повторная попытка через %v",
						consecutiveErrors, err, backoff)

					// Ждем с учетом контекста
					select {
					case <-time.After(backoff):
						// Продолжаем после ожидания
						logger.Debug("Таймаут ожидания перед повторной попыткой истек")
					case <-ctx.Done():
						logger.Info("Контекст завершен во время ожидания перезапуска")
						return
					}

					continue
				}

				consecutiveErrors = 0

				if ctx.Err() != nil {
					logger.Info("Контекст завершен, выходим из Consume")
					return
				}
			}

			select {
			case <-handlerReady:
				handlerReady = make(chan struct{})
				logger.Info("Перезапуск потребления после ребалансировки...")
				consecutiveErrors = 0
			case <-ctx.Done():
				logger.Info("Контекст завершен, выходим из Consume")
				return
			}
		}
	}()
}

// Stop останавливает консьюмера и ожидает завершения всех горутин
func (c *Consumer) Stop() {
	logger.Info("Остановка Kafka консьюмера")
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	if err := c.consumer.Close(); err != nil {
		logger.Errorf("Ошибка при закрытии консьюмера: %v", err)
	}
	logger.Info("Kafka консьюмер завершил работу")
}

// Setup выполняется при инициализации консьюмера
func (c *ConsumerHandler) Setup(session sarama.ConsumerGroupSession) error {
	logger.Infof("Консьюмер настроен: memberID=%s, generationID=%d", session.MemberID(), session.GenerationID())
	close(c.ready)
	return nil
}

// Cleanup выполняется при завершении работы консьюмера
func (c *ConsumerHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	logger.Infof("Консьюмер очищен: memberID=%s", session.MemberID())
	return nil
}

// ConsumeClaim обрабатывает сообщения из одного раздела (partition)
func (h *ConsumerHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	logger.Infof("Начало обработки партиции %d, начиная с offset %d",
		claim.Partition(), claim.InitialOffset())

	ctx := sess.Context()
	for {
		select {
		case <-ctx.Done():
			logger.Info("Контекст завершен, выходим из ConsumeClaim")
			return nil
		case message, ok := <-claim.Messages():
			if !ok {
				logger.Info("Канал сообщений закрыт, выходим из цикла обработки")
				return nil
			}

			logger.Debugf("Получено сообщение: topic=%s, partition=%d, offset=%d",
				message.Topic, message.Partition, message.Offset)

			var auditLog model.AuditLog
			if err := json.Unmarshal(message.Value, &auditLog); err != nil {
				logger.Errorf("Ошибка десериализации аудит-лога: %v", err)
				logger.Debugf("Содержимое сообщения: %s", string(message.Value))
				sess.MarkMessage(message, "")
				continue
			}

			// Добавим метаданные Kafka к логу для более полного представления
			enrichedLog := struct {
				model.AuditLog
				KafkaMeta struct {
					Topic     string `json:"topic"`
					Partition int32  `json:"partition"`
					Offset    int64  `json:"offset"`
					Key       string `json:"key,omitempty"`
				} `json:"kafka_meta"`
			}{
				AuditLog: auditLog,
			}

			enrichedLog.KafkaMeta.Topic = message.Topic
			enrichedLog.KafkaMeta.Partition = message.Partition
			enrichedLog.KafkaMeta.Offset = message.Offset
			if len(message.Key) > 0 {
				enrichedLog.KafkaMeta.Key = string(message.Key)
			}

			// Форматированный вывод через json.MarshalIndent
			data, err := json.MarshalIndent(enrichedLog, "", "  ")
			if err != nil {
				logger.Errorf("Ошибка маршалинга аудит-лога: %v", err)
				sess.MarkMessage(message, "")
				continue
			}

			logger.Infof("[KAFKA-AUDIT] Получено новое сообщение:\n%s", string(data))

			// Подтверждаем обработку сообщения
			sess.MarkMessage(message, "")
			logger.Debugf("Сообщение отмечено как обработанное: topic=%s, partition=%d, offset=%d",
				message.Topic, message.Partition, message.Offset)
		}
	}
}
