package pkg_test

import (
	"slices"
	"testing"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/pkg"
)

func TestNormalizeStringListNilAndEmptySemantics(t *testing.T) {
	if got := pkg.NormalizeStringList(nil, pkg.TrimLower, pkg.PreserveOrder); got != nil {
		t.Fatalf("nil input: expected nil, got %v", got.Values())
	}

	empty := cxlist.NewList[string]()
	if got := pkg.NormalizeStringList(empty, pkg.TrimLower, pkg.PreserveOrder); got != nil {
		t.Fatalf("empty input: expected nil, got %v", got.Values())
	}

	got := pkg.NormalizeStringList(cxlist.NewList(" ", "\t"), pkg.TrimLower, pkg.PreserveOrder)
	assertListValues(t, got, []string{})
}

func TestNormalizeStringListOrderAndDistinctSemantics(t *testing.T) {
	values := cxlist.NewList(" B ", "a", "b", " ", "A")

	preserved := pkg.NormalizeStringList(values, pkg.TrimLower, pkg.PreserveOrder)
	assertListValues(t, preserved, []string{"b", "a"})

	sorted := pkg.NormalizeStringList(values, pkg.TrimLower, pkg.SortStrings)
	assertListValues(t, sorted, []string{"a", "b"})

	assertListValues(t, values, []string{" B ", "a", "b", " ", "A"})
}

func TestNormalizePositiveIntListSemantics(t *testing.T) {
	if got := pkg.NormalizePositiveIntList(nil); got != nil {
		t.Fatalf("nil input: expected nil, got %v", got.Values())
	}

	empty := cxlist.NewList[int]()
	if got := pkg.NormalizePositiveIntList(empty); got != nil {
		t.Fatalf("empty input: expected nil, got %v", got.Values())
	}

	filtered := pkg.NormalizePositiveIntList(cxlist.NewList(0, -1, -2))
	assertListValues(t, filtered, []int{})

	normalized := pkg.NormalizePositiveIntList(cxlist.NewList(3, -1, 2, 3, 0, 1, 2))
	assertListValues(t, normalized, []int{1, 2, 3})
}

func TestNormalizeCSVListsRemainNonNilWhenBlank(t *testing.T) {
	stringsResult := pkg.NormalizeCSVStrings("  ", pkg.TrimLower, pkg.PreserveOrder)
	assertListValues(t, stringsResult, []string{})

	intsResult := pkg.ParsePositiveIntCSV("  ")
	assertListValues(t, intsResult, []int{})
}

func TestNormalizeCSVLists(t *testing.T) {
	stringsResult := pkg.NormalizeCSVStrings(" B, a,b, ,A", pkg.TrimLower, pkg.PreserveOrder)
	assertListValues(t, stringsResult, []string{"b", "a"})

	intsResult := pkg.ParsePositiveIntCSV("3,-1,invalid,2,3,0,1")
	assertListValues(t, intsResult, []int{1, 2, 3})
}

func assertListValues[T comparable](t *testing.T, got *cxlist.List[T], want []T) {
	t.Helper()
	if got == nil {
		t.Fatalf("expected non-nil list containing %v", want)
	}
	if values := got.Values(); !slices.Equal(values, want) {
		t.Fatalf("expected %v, got %v", want, values)
	}
}
