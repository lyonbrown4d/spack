//go:build spack_libvips

package pipeline

import (
	"context"
	"errors"
	"testing"
)

func TestImageMemoryBudgetAcquireContextHonorsCancellation(t *testing.T) {
	budget := newImageMemoryBudget(10)
	release, err := budget.Acquire(10)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = budget.AcquireContext(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestImageMemoryBudgetAcquireRejectsOversizedRequest(t *testing.T) {
	budget := newImageMemoryBudget(10)

	_, err := budget.AcquireContext(context.Background(), 11)
	if !IsVariantSkipped(err) {
		t.Fatalf("expected oversized memory request to be skipped, got %v", err)
	}
}
