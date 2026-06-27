package spackbundle

import "context"

// BundleWriter writes SPACK bundles.
type BundleWriter interface {
	Write(context.Context, WriteOptions) (WriteSummary, error)
}

// BundleExtractor extracts SPACK bundles for runtime serving.
type BundleExtractor interface {
	Extract(context.Context, string) (Extracted, error)
	ExtractReadOnly(context.Context, string) (Extracted, error)
}

// BundleReaderFactory opens SPACK bundle readers and reads bundle metadata.
type BundleReaderFactory interface {
	OpenReader(string) (*Reader, error)
	ReadIndex(string) (Index, error)
}

type bundleService struct{}

func newBundleService() bundleService {
	return bundleService{}
}

func (bundleService) Write(ctx context.Context, options WriteOptions) (WriteSummary, error) {
	return Write(ctx, options)
}

func (bundleService) Extract(ctx context.Context, bundlePath string) (Extracted, error) {
	return Extract(ctx, bundlePath)
}

func (bundleService) ExtractReadOnly(ctx context.Context, bundlePath string) (Extracted, error) {
	return ExtractReadOnly(ctx, bundlePath)
}

func (bundleService) OpenReader(bundlePath string) (*Reader, error) {
	return OpenReader(bundlePath)
}

func (bundleService) ReadIndex(bundlePath string) (Index, error) {
	return ReadIndex(bundlePath)
}

func newBundleWriter(service bundleService) BundleWriter {
	return service
}

func newBundleExtractor(service bundleService) BundleExtractor {
	return service
}

func newBundleReaderFactory(service bundleService) BundleReaderFactory {
	return service
}
