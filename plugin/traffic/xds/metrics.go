package xds

import (
	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// ClusterGauge is the number of clusters we are currently tracking.
	ClusterGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "traffic",
		Name:      "clusters_tracked",
		Help:      "Gauge of tracked clusters.",
	})
	EndpointGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: plugin.Namespace,
		Subsystem: "traffic",
		Name:      "endpoints_tracked",
		Help:      "Gauge of tracked endpoints.",
	})
)
