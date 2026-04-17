package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "authentik_operator_reconcile_total",
			Help: "Total number of reconciliation attempts by CRD type and result.",
		},
		[]string{"crd", "result"},
	)

	reconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "authentik_operator_reconcile_duration_seconds",
			Help:    "Duration of reconciliation attempts by CRD type.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"crd"},
	)
)

func init() {
	metrics.Registry.MustRegister(reconcileTotal, reconcileDuration)
}
