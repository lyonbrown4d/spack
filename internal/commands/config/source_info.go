package configcmd

import (
	"strings"

	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

const redactedPathValue = "REDACTED"

func effectiveSourceInfo(root string, redact bool) (map[string]any, error) {
	return newSourceInfoUseCase(source.NewResolver()).Info(root, redact)
}

type sourceInfoUseCase struct {
	resolver *source.Resolver
}

func newSourceInfoUseCase(resolver *source.Resolver) sourceInfoUseCase {
	if resolver == nil {
		resolver = source.NewResolver()
	}
	return sourceInfoUseCase{resolver: resolver}
}

func (u sourceInfoUseCase) Info(root string, redact bool) (map[string]any, error) {
	resolved, err := u.resolver.Resolve(root)
	if err != nil {
		return nil, oops.Wrapf(err, "resolve source info")
	}
	out := map[string]any{
		"root":          redactedPath(root, redact),
		"root_resolved": redactedPath(resolved.Root, redact),
		"type":          string(resolved.Type),
	}
	mo.PointerToOption(resolved.Bundle).ForEach(func(bundle source.BundleMetadata) {
		out["bundle"] = sourceBundleInfo(&bundle)
	})
	return out, nil
}

func sourceBundleInfo(bundle *source.BundleMetadata) map[string]any {
	return map[string]any{
		"format_version": bundle.FormatVersion,
		"index_kind":     bundle.IndexKind,
		"created_at":     bundle.CreatedAt,
		"file_count":     bundle.FileCount,
		"total_bytes":    bundle.TotalBytes,
	}
}

func redactedPath(path string, redact bool) string {
	if redact && strings.TrimSpace(path) != "" {
		return redactedPathValue
	}
	return path
}
