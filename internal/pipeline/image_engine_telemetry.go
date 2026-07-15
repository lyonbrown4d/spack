//go:build spack_libvips

package pipeline

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/arcgolabs/observabilityx"
	"github.com/samber/lo"
)

var (
	imageEngineOperationsTotalSpec = observabilityx.NewCounterSpec(
		"image_engine_operations_total",
		observabilityx.WithDescription("Total number of image engine operations."),
		observabilityx.WithLabelKeys("engine", "operation", "result"),
	)
	imageEngineOperationDurationSpec = observabilityx.NewHistogramSpec(
		"image_engine_operation_duration_seconds",
		observabilityx.WithDescription("Image engine operation duration in seconds."),
		observabilityx.WithUnit("s"),
		observabilityx.WithLabelKeys("engine", "operation", "result"),
	)
	imageEngineSourceBytesTotalSpec = observabilityx.NewCounterSpec(
		"image_engine_source_bytes_total",
		observabilityx.WithDescription("Total source image bytes processed by image engines."),
		observabilityx.WithUnit("By"),
		observabilityx.WithLabelKeys("engine"),
	)
	imageEngineSourcePixelsTotalSpec = observabilityx.NewCounterSpec(
		"image_engine_source_pixels_total",
		observabilityx.WithDescription("Total source image pixels processed by image engines."),
		observabilityx.WithLabelKeys("engine"),
	)
	imageEngineOutputBytesTotalSpec = observabilityx.NewCounterSpec(
		"image_engine_output_bytes_total",
		observabilityx.WithDescription("Total image output bytes produced by image engines."),
		observabilityx.WithUnit("By"),
		observabilityx.WithLabelKeys("engine", "target_format"),
	)
	imageEngineSavedBytesTotalSpec = observabilityx.NewCounterSpec(
		"image_engine_saved_bytes_total",
		observabilityx.WithDescription("Total image bytes saved by generated image variants."),
		observabilityx.WithUnit("By"),
		observabilityx.WithLabelKeys("engine", "target_format"),
	)
	imageEngineSavingRatioSpec = observabilityx.NewHistogramSpec(
		"image_engine_saving_ratio",
		observabilityx.WithDescription("Image variant saving ratio."),
		observabilityx.WithLabelKeys("engine", "target_format"),
	)
	imageEngineSkipsTotalSpec = observabilityx.NewCounterSpec(
		"image_engine_skips_total",
		observabilityx.WithDescription("Total number of skipped image engine operations."),
		observabilityx.WithLabelKeys("engine", "reason"),
	)
	imageEngineSkipReasonRules = []struct {
		contains string
		reason   string
	}{
		{contains: "no_variants", reason: "no_variants"},
		{contains: "max source bytes", reason: "source_bytes"},
		{contains: "max source pixels", reason: "source_pixels"},
		{contains: "max memory bytes", reason: "memory_budget"},
		{contains: "max width", reason: "source_width"},
		{contains: "max height", reason: "source_height"},
		{contains: "dimensions", reason: "source_dimensions"},
	}
)

type imageEngineTelemetry struct {
	obs observabilityx.Observability
}

func newImageEngineTelemetry(logger *slog.Logger, obs observabilityx.Observability) imageEngineTelemetry {
	return imageEngineTelemetry{obs: observabilityx.Normalize(obs, logger)}
}

func (t imageEngineTelemetry) recordOperation(engine, operation, result string, startedAt time.Time) {
	if t.obs == nil {
		return
	}
	attrs := []observabilityx.Attribute{
		observabilityx.String("engine", strings.TrimSpace(engine)),
		observabilityx.String("operation", strings.TrimSpace(operation)),
		observabilityx.String("result", strings.TrimSpace(result)),
	}
	ctx := context.TODO()
	t.obs.Counter(imageEngineOperationsTotalSpec).Add(ctx, 1, attrs...)
	t.obs.Histogram(imageEngineOperationDurationSpec).Record(ctx, time.Since(startedAt).Seconds(), attrs...)
}

func (t imageEngineTelemetry) recordSource(engine string, sourceBytes int64, width, height int) {
	if t.obs == nil {
		return
	}
	attrs := []observabilityx.Attribute{observabilityx.String("engine", strings.TrimSpace(engine))}
	ctx := context.TODO()
	if sourceBytes > 0 {
		t.obs.Counter(imageEngineSourceBytesTotalSpec).Add(ctx, sourceBytes, attrs...)
	}
	pixels := int64(width) * int64(height)
	if pixels > 0 {
		t.obs.Counter(imageEngineSourcePixelsTotalSpec).Add(ctx, pixels, attrs...)
	}
}

func (t imageEngineTelemetry) recordVariant(
	engine string,
	targetFormat string,
	outputBytes int64,
	savedBytes int64,
	savingRatio float64,
) {
	if t.obs == nil {
		return
	}
	attrs := []observabilityx.Attribute{
		observabilityx.String("engine", strings.TrimSpace(engine)),
		observabilityx.String("target_format", strings.TrimSpace(targetFormat)),
	}
	ctx := context.TODO()
	if outputBytes > 0 {
		t.obs.Counter(imageEngineOutputBytesTotalSpec).Add(ctx, outputBytes, attrs...)
	}
	if savedBytes > 0 {
		t.obs.Counter(imageEngineSavedBytesTotalSpec).Add(ctx, savedBytes, attrs...)
	}
	t.obs.Histogram(imageEngineSavingRatioSpec).Record(ctx, savingRatio, attrs...)
}

func (t imageEngineTelemetry) recordSkip(engine, reason string) {
	if t.obs == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || reason == "ok" {
		return
	}
	t.obs.Counter(imageEngineSkipsTotalSpec).Add(
		context.TODO(),
		1,
		observabilityx.String("engine", strings.TrimSpace(engine)),
		observabilityx.String("reason", reason),
	)
}

func imageEngineResult(err error) string {
	if err == nil {
		return "ok"
	}
	if IsVariantSkipped(err) {
		return "skipped"
	}
	return "error"
}

func imageEngineSkipReason(err error) string {
	if err == nil {
		return "ok"
	}
	if !IsVariantSkipped(err) {
		return "error"
	}
	message := err.Error()
	rule, ok := lo.Find(imageEngineSkipReasonRules, func(rule struct {
		contains string
		reason   string
	}) bool {
		return strings.Contains(message, rule.contains)
	})
	if ok {
		return rule.reason
	}
	return "skipped"
}
