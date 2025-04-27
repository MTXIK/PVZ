package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Бизнес метрики
	OrdersAccepted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pvz_orders_accepted_total",
		Help: "Общее количество принятых заказов",
	})

	OrdersDelivered = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pvz_orders_delivered_total",
		Help: "Общее количество доставленных заказов клиентам",
	})

	OrdersReturned = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pvz_orders_returned_total",
		Help: "Общее количество возвращенных заказов",
	})

	OrdersReturnedToCourier = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pvz_orders_returned_to_courier_total",
		Help: "Общее количество заказов, возвращенных курьеру",
	})

	OrdersProcessingTime = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pvz_order_processing_seconds",
		Help:    "Время обработки заказов (от принятия до доставки)",
		Buckets: prometheus.LinearBuckets(1, 60, 10), // от 1 до 10 минут, с шагом 1 минута
	})

	// Технические метрики
	HttpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Общее количество HTTP-запросов",
		},
		[]string{"method", "path", "status"},
	)

	HttpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Длительность HTTP-запросов",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)
