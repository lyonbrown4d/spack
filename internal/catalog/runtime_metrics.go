package catalog

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type RuntimeMetrics struct {
	AssetsCurrent                  prometheus.Gauge
	VariantsCurrent                prometheus.Gauge
	SourceBytesCurrent             prometheus.Gauge
	CatalogScanDuration            prometheus.Gauge
	SourceMode                     *prometheus.GaugeVec
	SourceBundleExtractionDuration prometheus.Gauge
	SourceBundleExtractionFiles    prometheus.Gauge
	SourceBundleExtractionBytes    prometheus.Gauge
}

func NewRuntimeMetrics() *RuntimeMetrics {
	metrics := &RuntimeMetrics{
		AssetsCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_catalog_assets_current",
			Help: "Current number of assets tracked by the in-memory catalog",
		}),
		VariantsCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_catalog_variants_current",
			Help: "Current number of variants tracked by the in-memory catalog",
		}),
		SourceBytesCurrent: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_catalog_source_bytes_current",
			Help: "Current total source bytes observed during the latest catalog scan",
		}),
		CatalogScanDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_catalog_scan_duration_seconds",
			Help: "Duration of the latest successful catalog scan in seconds",
		}),
		SourceMode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "spack_source_mode_current",
			Help: "Current asset source mode for this runtime instance",
		}, []string{"mode"}),
		SourceBundleExtractionDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_source_bundle_extraction_duration_seconds",
			Help: "Duration of the initial .spack source bundle extraction in seconds",
		}),
		SourceBundleExtractionFiles: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_source_bundle_extraction_files",
			Help: "Number of files in the extracted .spack source bundle",
		}),
		SourceBundleExtractionBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "spack_source_bundle_extraction_bytes",
			Help: "Total bytes described by the extracted .spack source bundle index",
		}),
	}
	metrics.SetSourceMode("unknown")
	return metrics
}

func (m *RuntimeMetrics) Collectors() []prometheus.Collector {
	if m == nil {
		return nil
	}
	return []prometheus.Collector{
		m.AssetsCurrent,
		m.VariantsCurrent,
		m.SourceBytesCurrent,
		m.CatalogScanDuration,
		m.SourceMode,
		m.SourceBundleExtractionDuration,
		m.SourceBundleExtractionFiles,
		m.SourceBundleExtractionBytes,
	}
}

func (m *RuntimeMetrics) RecordCatalogScan(duration time.Duration, cat Catalog, totalBytes int64) {
	if m == nil {
		return
	}
	m.SetScanDuration(duration)
	m.SyncCatalog(cat)
	m.SetSourceBytes(totalBytes)
}

func (m *RuntimeMetrics) SyncCatalog(cat Catalog) {
	if m == nil || cat == nil {
		return
	}
	m.AssetsCurrent.Set(float64(cat.AssetCount()))
	m.VariantsCurrent.Set(float64(cat.VariantCount()))
}

func (m *RuntimeMetrics) SetSourceBytes(totalBytes int64) {
	if m == nil {
		return
	}
	m.SourceBytesCurrent.Set(float64(totalBytes))
}

func (m *RuntimeMetrics) SetScanDuration(duration time.Duration) {
	if m == nil {
		return
	}
	if duration < 0 {
		duration = 0
	}
	m.CatalogScanDuration.Set(duration.Seconds())
}

func (m *RuntimeMetrics) SetSourceStats(mode string, extractionDuration time.Duration, files int, bytes int64) {
	if m == nil {
		return
	}
	m.SetSourceMode(mode)
	if extractionDuration < 0 {
		extractionDuration = 0
	}
	if files < 0 {
		files = 0
	}
	if bytes < 0 {
		bytes = 0
	}
	m.SourceBundleExtractionDuration.Set(extractionDuration.Seconds())
	m.SourceBundleExtractionFiles.Set(float64(files))
	m.SourceBundleExtractionBytes.Set(float64(bytes))
}

func (m *RuntimeMetrics) SetSourceMode(mode string) {
	if m == nil || m.SourceMode == nil {
		return
	}
	selected := normalizeSourceMode(mode)
	for _, candidate := range sourceModeValues {
		m.SourceMode.WithLabelValues(candidate).Set(0)
	}
	m.SourceMode.WithLabelValues(selected).Set(1)
}

var sourceModeValues = []string{"direct", "aot", "unknown"}

func normalizeSourceMode(mode string) string {
	normalized := strings.TrimSpace(strings.ToLower(mode))
	switch normalized {
	case "direct", "aot":
		return normalized
	default:
		return "unknown"
	}
}
