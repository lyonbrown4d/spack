package pipeline_test

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/pipeline"
	"slices"
	"testing"
)

func TestNormalizeEncodingsPreservesFirstSeenOrder(t *testing.T) {
	got := pipeline.NormalizeEncodingsForTest(cxlist.NewList(" gzip ", "br", "gzip", "zstd", "bad", "br"))
	want := []string{"gzip", "br", "zstd"}
	if !slices.Equal(got.Values(), want) {
		t.Fatalf("expected encodings %#v, got %#v", want, got)
	}
}

func TestNormalizeImageFormatsPreservesFirstSeenOrder(t *testing.T) {
	got := pipeline.NormalizeImageFormatsForTest(cxlist.NewList(" png ", "jpeg", "jpg", "png", "bad"))
	want := []string{"png", "jpeg"}
	if !slices.Equal(got.Values(), want) {
		t.Fatalf("expected formats %#v, got %#v", want, got)
	}
}

func TestNormalizeRequestStringsSortsAndDeduplicates(t *testing.T) {
	got := pipeline.NormalizeRequestStringsForTest(cxlist.NewList(" gzip ", "br", "gzip", "zstd", "", " BR "))
	want := []string{"br", "gzip", "zstd"}
	if !slices.Equal(got.Values(), want) {
		t.Fatalf("expected request strings %#v, got %#v", want, got)
	}
}

func TestNormalizeRequestIntsSortsAndDeduplicates(t *testing.T) {
	got := pipeline.NormalizeRequestIntsForTest(cxlist.NewList(1280, 640, 1280, 0, -1))
	want := []int{640, 1280}
	if !slices.Equal(got.Values(), want) {
		t.Fatalf("expected request ints %#v, got %#v", want, got)
	}
}
