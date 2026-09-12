package asyncx

import (
	"context"
	"testing"
)

func TestJoinCollectedErrorsSuccessDoesNotAllocateForOneMillionItems(t *testing.T) {
	runErrors := make([]error, 1_000_000)
	allocations := testing.AllocsPerRun(1, func() {
		if err := joinCollectedErrors(context.Background(), runErrors); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("expected no allocations while collecting one million successful results, got %.0f", allocations)
	}
}

func BenchmarkJoinCollectedErrorsSuccessOneMillion(b *testing.B) {
	runErrors := make([]error, 1_000_000)
	b.ReportAllocs()
	for b.Loop() {
		if err := joinCollectedErrors(context.Background(), runErrors); err != nil {
			b.Fatal(err)
		}
	}
}
