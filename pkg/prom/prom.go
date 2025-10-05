package prom

import (
	"fmt"
	"sync"

	xhttp "github.com/nimasrn/message-gateway/pkg/http"
	"github.com/nimasrn/message-gateway/pkg/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
)

const (
	SystemMessages = "message"
)
const (
	MetricMessageExpressDeliveredDuration = "express_delivered_duration_seconds"
)

const (
	TypeCounter      = "counter"
	TypeCounterVec   = "counterVec"
	TypeHistogram    = "histogram"
	TypeHistogramVec = "histogramVec"
	TypeGaugeVec     = "gaugeVec"
)

var lockCreateMetricLock = &sync.Mutex{}
var namespace = "none"

var MetricSystemEnabled = false

var MetricCollectionCounters = make(map[string]prometheus.Counter)
var MetricCollectionCounterVec = make(map[string]*prometheus.CounterVec)
var MetricCollectionGaugeVec = make(map[string]*prometheus.GaugeVec)
var MetricCollectionHistogram = make(map[string]prometheus.Histogram)
var MetricCollectionHistogramVec = make(map[string]*prometheus.HistogramVec)

var defaultLabels prometheus.Labels

func Create(host string, env string, nameSpace string) error {
	defaultLabels = make(prometheus.Labels)
	defaultLabels["env"] = env
	defaultLabels["instance"] = host
	namespace = nameSpace
	MetricSystemEnabled = true

	var err error
	hasError := func(e error) {
		if err == nil && e != nil {
			err = e
		}
	}

	// Messages
	hasError(createHistogramVec(SystemMessages, MetricMessageExpressDeliveredDuration, []string{"priority"}))

	return err
}

func CreateMetric(metricType, metricSubsystem, metricName string, labelsValues ...string) error {
	switch metricType {
	case TypeCounter:
		return createCounter(metricSubsystem, metricName)
	case TypeCounterVec:
		return createCounterVec(metricSubsystem, metricName, labelsValues)
	case TypeHistogram:
		return createHistogram(metricSubsystem, metricName)
	case TypeHistogramVec:
		return createHistogramVec(metricSubsystem, metricName, labelsValues)
	case TypeGaugeVec:
		return createGaugeVec(metricSubsystem, metricName, labelsValues)
	}
	return fmt.Errorf("metric type %s is not defined", metricType)
}

func ListenAndServer(port string, url string) {
	hh := fasthttpadaptor.NewFastHTTPHandler(promhttp.Handler())
	s := xhttp.CreateServer()
	s.GET(url, hh)
	logger.Info("[metrics-server] listening...", "url", url)
	if err := s.ListenAndServe(port); err != nil {
		logger.Panic("[metrics-server] http listen error", "error", err)
	}
}

func createCounter(subsystem, name string) error {
	lockCreateMetricLock.Lock()
	defer lockCreateMetricLock.Unlock()
	MetricCollectionCounters[subsystem+name] = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        "",
		ConstLabels: defaultLabels,
	})
	return prometheus.Register(MetricCollectionCounters[subsystem+name])
}

func createCounterVec(subsystem, name string, labels []string) error {
	lockCreateMetricLock.Lock()
	defer lockCreateMetricLock.Unlock()
	MetricCollectionCounterVec[subsystem+name] = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        "",
		ConstLabels: defaultLabels,
	}, labels)
	return prometheus.Register(MetricCollectionCounterVec[subsystem+name])
}

func createHistogram(subsystem, name string) error {
	lockCreateMetricLock.Lock()
	defer lockCreateMetricLock.Unlock()
	MetricCollectionHistogram[subsystem+name] = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        "",
		ConstLabels: defaultLabels,
		Buckets:     prometheus.DefBuckets,
	})
	return prometheus.Register(MetricCollectionHistogram[subsystem+name])
}

func createHistogramVec(subsystem, name string, labels []string) error {
	lockCreateMetricLock.Lock()
	defer lockCreateMetricLock.Unlock()
	MetricCollectionHistogramVec[subsystem+name] = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        "",
		ConstLabels: defaultLabels,
	}, labels)
	return prometheus.Register(MetricCollectionHistogramVec[subsystem+name])
}

func createHistogramWithLabelVal(subsystem, name string, label string, value string) error {
	lockCreateMetricLock.Lock()
	defer lockCreateMetricLock.Unlock()
	var l prometheus.Labels
	l = make(map[string]string)
	for k, v := range defaultLabels {
		l[k] = v
	}
	l[label] = value
	MetricCollectionHistogram[subsystem+name+label+value] = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        "",
		ConstLabels: l,
	})
	return prometheus.Register(MetricCollectionHistogram[subsystem+name+label+value])
}

func createGaugeVec(subsystem, name string, labels []string) error {
	lockCreateMetricLock.Lock()
	defer lockCreateMetricLock.Unlock()

	MetricCollectionGaugeVec[subsystem+name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        name,
		Help:        "",
		ConstLabels: defaultLabels,
	}, labels)
	return prometheus.Register(MetricCollectionGaugeVec[subsystem+name])
}

func IncCounter(subsystem, name string) {
	AddCounter(subsystem, name, 1)
}

func AddCounter(subsystem, name string, number float64) {
	if MetricSystemEnabled == false {
		return
	}
	if v, ok := MetricCollectionCounters[subsystem+name]; ok {
		v.Add(number)
		return
	}
	logger.Warn("[metrics-server] counter not found", "subsystem", subsystem, "name", name)
}

func IncGaugeVec(subsystem, name string, labelValues ...string) {
	AddGaugeVec(subsystem, name, 1, labelValues...)
}

func AddGaugeVec(subsystem, name string, num float64, labelValues ...string) {
	if MetricSystemEnabled == false {
		return
	}
	if v, ok := MetricCollectionGaugeVec[subsystem+name]; ok {
		v.WithLabelValues(labelValues...).Add(num)
		return
	}
	logger.Warn("[metrics-server] gauge not found", "subsystem", subsystem, "name", name)
}

func AddCounterVec(subsystem, name string, num float64, labelValues ...string) {
	if MetricSystemEnabled == false {
		return
	}
	if v, ok := MetricCollectionCounterVec[subsystem+name]; ok {
		v.WithLabelValues(labelValues...).Add(num)
		return
	}
	logger.Warn("[metrics-server] counter vec not found", "subsystem", subsystem, "name", name)
}

func IncCounterVec(subsystem, name string, labelValues ...string) {
	AddCounterVec(subsystem, name, 1, labelValues...)
}

func AddHistogram(subsystem, name string, number float64) {
	if MetricSystemEnabled == false {
		return
	}
	if v, ok := MetricCollectionHistogram[subsystem+name]; ok {
		v.Observe(number)
		return
	}
	logger.Warn("[metrics-server] histogram not found", "subsystem", subsystem, "name", name)
}

func AddHistogramVec(subsystem, name string, number float64, labelValues ...string) {
	if MetricSystemEnabled == false {
		return
	}
	if v, ok := MetricCollectionHistogramVec[subsystem+name]; ok {
		v.WithLabelValues(labelValues...).Observe(number)
		return
	}
	logger.Warn("[metrics-server] histogram vec not found", "subsystem", subsystem, "name", name)
}

func AddMessageExpressDeliveryDuration(duration float64, priority string) {
	AddHistogramVec(SystemMessages, MetricMessageExpressDeliveredDuration, duration, priority)
}
