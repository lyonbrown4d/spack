package spackbundle_test

import (
	"testing"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/spack/internal/spackbundle"
)

func TestModuleResolvesBundleServices(t *testing.T) {
	app := dix.New("spackbundle-test", dix.Modules(spackbundle.Module))
	if err := app.Validate(); err != nil {
		t.Fatalf("validate module: %v", err)
	}
	runtime, err := app.Build()
	if err != nil {
		t.Fatalf("build module: %v", err)
	}
	if _, err := dix.ResolveAs[spackbundle.BundleWriter](runtime.Container()); err != nil {
		t.Fatalf("resolve bundle writer: %v", err)
	}
	if _, err := dix.ResolveAs[spackbundle.BundleExtractor](runtime.Container()); err != nil {
		t.Fatalf("resolve bundle extractor: %v", err)
	}
	if _, err := dix.ResolveAs[spackbundle.BundleReaderFactory](runtime.Container()); err != nil {
		t.Fatalf("resolve bundle reader factory: %v", err)
	}
}
