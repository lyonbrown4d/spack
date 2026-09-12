package server_test

import (
	"testing"
	"time"

	"github.com/lyonbrown4d/spack/internal/server"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRuntimeMetricsRecordPreparedSnapshotRebuildHealth(t *testing.T) {
	metrics := server.NewRuntimeMetrics()

	metrics.RecordPreparedSnapshotRebuild(2*time.Second, "success")
	metrics.RecordPreparedSnapshotRebuild(-time.Second, "failure")
	metrics.SetPreparedSnapshotRebuildWorkerRunning(true)
	metrics.IncPreparedSnapshotRebuildCoalesced()
	metrics.IncPreparedSnapshotRebuildCoalesced()

	if got := testutil.ToFloat64(metrics.PreparedSnapshotRebuilds.WithLabelValues("success")); got != 1 {
		t.Fatalf("expected one successful rebuild, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.PreparedSnapshotRebuilds.WithLabelValues("error")); got != 1 {
		t.Fatalf("expected one failed rebuild, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.PreparedSnapshotRebuildWorkerRunning); got != 1 {
		t.Fatalf("expected rebuild worker running gauge 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.PreparedSnapshotRebuildCoalescedTotal); got != 2 {
		t.Fatalf("expected two coalesced rebuilds, got %v", got)
	}

	metrics.SetPreparedSnapshotRebuildWorkerRunning(false)
	if got := testutil.ToFloat64(metrics.PreparedSnapshotRebuildWorkerRunning); got != 0 {
		t.Fatalf("expected rebuild worker running gauge 0, got %v", got)
	}
}
