package spackbundle

import "github.com/samber/oops"

const (
	// maxBundleIndexBytes covers 1,000,000 representative AOT entries with measured headroom.
	maxBundleIndexBytes    int64 = 768 << 20
	maxBundleFiles               = 1_000_000
	maxExpandedBundleBytes int64 = 64 << 30
	maxExtractedFileBytes  int64 = 2 << 30
)

type bundleLimits struct {
	indexBytes    int64
	files         int
	expandedBytes int64
	fileBytes     int64
}

var productionBundleLimits = bundleLimits{
	indexBytes:    maxBundleIndexBytes,
	files:         maxBundleFiles,
	expandedBytes: maxExpandedBundleBytes,
	fileBytes:     maxExtractedFileBytes,
}

type bundleBudget struct {
	limits        bundleLimits
	files         int
	expandedBytes int64
}

func validateBundleFileCount(count int, limits bundleLimits) error {
	if count < 0 || count > limits.files {
		return oops.Errorf("bundle exceeds max file count: %d", limits.files)
	}
	return nil
}

func (budget *bundleBudget) add(size int64) error {
	if size < 0 {
		return oops.In("spackbundle").Owner("limits").Errorf("bundle entry has negative size: %d", size)
	}
	if budget.files >= budget.limits.files {
		return oops.Errorf("bundle exceeds max file count: %d", budget.limits.files)
	}
	if size > budget.limits.expandedBytes-budget.expandedBytes {
		return oops.Errorf("bundle exceeds max expanded bytes: %d", budget.limits.expandedBytes)
	}
	budget.files++
	budget.expandedBytes += size
	return nil
}

func validateBundleFiles(files []File, limits bundleLimits) error {
	if err := validateBundleFileCount(len(files), limits); err != nil {
		return err
	}
	budget := bundleBudget{limits: limits}
	for index := range files {
		if files[index].Size > limits.fileBytes {
			return oops.Errorf("bundle file %q exceeds max extracted bytes", files[index].Path)
		}
		if err := budget.add(files[index].Size); err != nil {
			return err
		}
	}
	return nil
}

func maxBundleEntryBytes(filePath string, limits bundleLimits) int64 {
	if filePath == IndexPath {
		return limits.indexBytes
	}
	return limits.fileBytes
}

func validateBundleEntrySize(filePath string, size, maxBytes int64) error {
	if size >= 0 && size <= maxBytes {
		return nil
	}
	if filePath == IndexPath {
		return oops.Errorf("bundle index exceeds max bytes: %d", maxBytes)
	}
	return oops.Errorf("bundle file %q exceeds max extracted bytes", filePath)
}
