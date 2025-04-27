package utils

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"gitlab.ozon.dev/gojhw1/pkg/logger"
	"gitlab.ozon.dev/gojhw1/pkg/model"
)

const (
	maxLogChanSize = 100
	numWorkerTypes = 2
)

type WorkerPool struct {
	processor    logProcessor
	workersNum   int
	batchSize    int
	batchTimeout time.Duration
	wg           sync.WaitGroup
	inputCh      chan model.AuditLog
}

type LogQueue struct {
	overflow   *list.List
	overflowWg sync.WaitGroup
	mu         sync.Mutex
}

type AuditLogger struct {
	mainLogCh   chan model.AuditLog
	fanOutWg    sync.WaitGroup
	cancel      context.CancelFunc
	logQueue    LogQueue
	workerPools []*WorkerPool
}

// NewAuditLogger создает новый экземпляр аудит-логгера
func NewAuditLogger(ctx context.Context, auditRepo auditRepository, workersNum, batchSize int, batchTimeout time.Duration) *AuditLogger {
	ctx, cancel := context.WithCancel(ctx)

	logger := &AuditLogger{
		workerPools: make([]*WorkerPool, 0, numWorkerTypes),
		mainLogCh:   make(chan model.AuditLog, maxLogChanSize),
		cancel:      cancel,
		logQueue: LogQueue{
			overflow: list.New(),
		},
	}

	stdoutLogProcessor := newStdoutLogProcessor()
	dbLogProcessor := newDBLogProcessor(auditRepo)

	stdoutPool := &WorkerPool{
		processor:    stdoutLogProcessor,
		workersNum:   workersNum,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		inputCh:      make(chan model.AuditLog, maxLogChanSize),
	}
	logger.workerPools = append(logger.workerPools, stdoutPool)

	dbPool := &WorkerPool{
		processor:    dbLogProcessor,
		workersNum:   workersNum,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		inputCh:      make(chan model.AuditLog, maxLogChanSize),
	}
	logger.workerPools = append(logger.workerPools, dbPool)

	for _, pool := range logger.workerPools {
		pool.wg.Add(pool.workersNum)
		for i := 0; i < pool.workersNum; i++ {
			go logger.worker(ctx, pool, fmt.Sprintf("%s-worker-%d", pool.processor.name(), i+1))
		}
	}

	logger.fanOutWg.Add(1)
	go logger.fanOutLogs(ctx)

	logger.logQueue.overflowWg.Add(1)
	go func() {
		defer logger.logQueue.overflowWg.Done()
		logger.processOverflow(ctx)
	}()

	return logger
}

// Shutdown завершает работу аудит-логгера
func (l *AuditLogger) Shutdown() {
	l.cancel()
	close(l.mainLogCh)

	l.fanOutWg.Wait()

	for _, pool := range l.workerPools {
		pool.wg.Wait()
	}

	l.logQueue.overflowWg.Wait()
}

// Log отправляет лог в канал
func (l *AuditLogger) Log(ctx context.Context, log model.AuditLog) {
	select {
	case l.mainLogCh <- log:
		// Успешно отправили
	case <-ctx.Done():
	default:
		l.logQueue.mu.Lock()
		defer l.logQueue.mu.Unlock()
		l.logQueue.overflow.PushBack(log)
	}
}

// LogOrderStatusChange логирует изменение статуса заказа
func (l *AuditLogger) LogOrderStatusChange(ctx context.Context, orderID int64, oldStatus, newStatus string) {
	l.Log(ctx, model.AuditLog{
		Timestamp: time.Now(),
		Type:      model.AuditLogTypeOrderStatus,
		OrderID:   orderID,
		OldStatus: oldStatus,
		NewStatus: newStatus,
	})
}

// fanOutLogs запускает горутину для распределения логов по всем пулам
func (l *AuditLogger) fanOutLogs(ctx context.Context) {
	defer l.fanOutWg.Done()

	for {
		select {
		case log, ok := <-l.mainLogCh:
			if !ok {
				for _, pool := range l.workerPools {
					close(pool.inputCh)
				}
				return
			}

			// Отправляем один и тот же лог во все пулы
			for _, pool := range l.workerPools {
				select {
				case pool.inputCh <- log:
					// Лог успешно отправлен в пул
				case <-ctx.Done():
					return
				}
			}

		case <-ctx.Done():
			// Контекст отменен, закрываем каналы всех пулов
			for _, pool := range l.workerPools {
				close(pool.inputCh)
			}
			return
		}
	}
}

// worker запускает воркер аудит-логов
func (l *AuditLogger) worker(ctx context.Context, pool *WorkerPool, name string) {
	defer pool.wg.Done()

	logger.Infof("[%s] Воркер аудит-логов запущен", name)

	batch := make([]model.AuditLog, 0, pool.batchSize)
	timer := time.NewTimer(pool.batchTimeout)
	defer timer.Stop()

	for {
		select {
		case logEntry, ok := <-pool.inputCh:
			if !ok {
				if len(batch) > 0 {
					err := pool.processor.processLogs(ctx, name, batch)
					if err != nil {
						logger.Errorf("[%s] Ошибка обработки пакета логов: %v", name, err)
					}
				}
				logger.Infof("[%s] Воркер завершен из-за закрытия канала", name)
				return
			}

			batch = append(batch, logEntry)

			if len(batch) >= pool.batchSize {
				err := pool.processor.processLogs(ctx, name, batch)
				if err != nil {
					logger.Errorf("[%s] Ошибка обработки пакета логов: %v", name, err)
				}
				batch = make([]model.AuditLog, 0, pool.batchSize)
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(pool.batchTimeout)
			}

		case <-timer.C:
			if len(batch) > 0 {
				err := pool.processor.processLogs(ctx, name, batch)
				if err != nil {
					logger.Errorf("[%s] Ошибка обработки пакета логов: %v", name, err)
				}
				batch = make([]model.AuditLog, 0, pool.batchSize)
			}
			timer.Reset(pool.batchTimeout)

		case <-ctx.Done():
			if len(batch) > 0 {
				err := pool.processor.processLogs(ctx, name, batch)
				if err != nil {
					logger.Errorf("[%s] Ошибка обработки пакета логов при завершении: %v", name, err)
				}
			}
			logger.Infof("[%s] Воркер аудит-логов завершен после отмены контекста", name)
			return
		}
	}
}

// processOverflow запускает цикл обработки очереди переполнения
func (l *AuditLogger) processOverflow(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			select {
			case <-ctx.Done():
				l.tryFlushOverflowOnShutdown()
				return
			default:
				l.tryFlushOverflow(ctx)
			}
		case <-ctx.Done():
			l.tryFlushOverflowOnShutdown()
			return
		}
	}
}

// tryFlushOverflow пытается отправить логи из очереди переполнения в основной канал
func (l *AuditLogger) tryFlushOverflow(ctx context.Context) {
	for {
		l.logQueue.mu.Lock()
		if l.logQueue.overflow.Len() == 0 {
			l.logQueue.mu.Unlock()
			return
		}

		front := l.logQueue.overflow.Front()
		log := front.Value.(model.AuditLog)
		l.logQueue.overflow.Remove(front)
		l.logQueue.mu.Unlock()

		select {
		case l.mainLogCh <- log:
		case <-ctx.Done():
			l.logQueue.mu.Lock()
			l.logQueue.overflow.PushFront(log)
			l.logQueue.mu.Unlock()
			return
		default:
			l.logQueue.mu.Lock()
			l.logQueue.overflow.PushFront(log)
			l.logQueue.mu.Unlock()
			return
		}
	}
}

// tryFlushOverflowOnShutdown выводит все оставшиеся логи из очереди переполнения
// напрямую при завершении работы логгера
func (l *AuditLogger) tryFlushOverflowOnShutdown() {
	l.logQueue.mu.Lock()
	if l.logQueue.overflow.Len() == 0 {
		l.logQueue.mu.Unlock()
		return
	}

	// Берем все логи сразу, чтобы не удерживать mutex слишком долго
	logs := make([]model.AuditLog, 0, l.logQueue.overflow.Len())
	for e := l.logQueue.overflow.Front(); e != nil; e = e.Next() {
		logs = append(logs, e.Value.(model.AuditLog))
	}
	l.logQueue.overflow.Init() // Очищаем список
	l.logQueue.mu.Unlock()

	logger.Infof("[AUDIT-OVERFLOW] Вывод %d оставшихся логов при завершении работы", len(logs))
	for _, myLog := range logs {
		data, err := json.MarshalIndent(myLog, "", "  ")
		if err != nil {
			logger.Errorf("[AUDIT-OVERFLOW-ERROR] Ошибка маршалинга лога: %v", err)
		}
		logger.Infof("[AUDIT-OVERFLOW] %s", string(data))
	}
}
