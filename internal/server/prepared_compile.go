package server

import (
	"cmp"
	"context"
	"github.com/samber/oops"
	"log/slog"
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/spack/internal/cachepolicy"
	"github.com/lyonbrown4d/spack/internal/catalog"
	"github.com/lyonbrown4d/spack/internal/config"
	"github.com/lyonbrown4d/spack/internal/media"
	"github.com/lyonbrown4d/spack/internal/resolver"
	"github.com/samber/lo"
)

type preparedCompiler struct {
	cfg           *config.Config
	policy        cachepolicy.ResponsePolicy
	memoryPolicy  cachepolicy.MemoryPolicy
	resourceHints *resourceHintService
	logger        *slog.Logger
	fileGuards    *serverFileGuards
	bodyBudget    *preparedBodyBudget
}

func newPreparedCompiler(
	cfg *config.Config,
	resourceHints *resourceHintService,
	logger *slog.Logger,
	fileGuards *serverFileGuards,
) preparedCompiler {
	return preparedCompiler{
		cfg:           cfg,
		policy:        cachepolicy.NewResponsePolicyFromConfig(cfg),
		memoryPolicy:  cachepolicy.NewMemoryPolicy(cfg),
		resourceHints: resourceHints,
		logger:        logger,
		fileGuards:    fileGuards,
		bodyBudget:    newPreparedBodyBudget(cfg),
	}
}

func (c preparedCompiler) Compile(ctx context.Context, cat catalog.Catalog) (*preparedSnapshot, error) {
	if err := preparedContextErr(ctx); err != nil {
		return nil, err
	}
	catalogSnapshot := cat.Snapshot()
	snapshot := newPreparedSnapshot(catalogSnapshot.Assets.Len())
	c.compileRoutes(catalogSnapshot.Assets).Each(func(_ int, route *preparedRoute) {
		snapshot.routes.Set(route.path, route)
		if alias := preparedEntryAlias(route.path, c.cfg.Assets.Entry); alias != "" {
			if _, exists := snapshot.routes.Get(alias); !exists {
				snapshot.routes.Set(alias, route)
			}
		}
		bodyEntries, bodyBytes := routePreparedBodyStats(route)
		snapshot.assets++
		snapshot.variants += routeVariantCount(route)
		snapshot.bodyEntries += bodyEntries
		snapshot.bodyBytes += bodyBytes
	})
	return snapshot, nil
}

func (c preparedCompiler) compileRoutes(entries *cxlist.List[*catalog.Entry]) *cxlist.List[*preparedRoute] {
	return cxlist.FilterMapList[*catalog.Entry, *preparedRoute](entries, func(_ int, entry *catalog.Entry) (*preparedRoute, bool) {
		route := c.compileRoute(entry)
		return route, route != nil
	})
}

func (c preparedCompiler) compileRoute(entry *catalog.Entry) *preparedRoute {
	if entry == nil || entry.Asset == nil {
		return nil
	}

	identity := c.compileAssetResponse(entry.Asset)
	if identity == nil {
		return nil
	}
	route := newPreparedRoute(entry.Asset.Path, identity)
	c.compileVariantResponses(entry.Asset, entry.Variants).Each(func(_ int, response *preparedResponse) {
		route.addVariant(response)
	})
	route.finalize()
	return route
}

func (c preparedCompiler) compileVariantResponses(
	asset *catalog.Asset,
	variants *cxlist.List[*catalog.Variant],
) *cxlist.List[*preparedResponse] {
	if variants == nil {
		return cxlist.NewList[*preparedResponse]()
	}
	return cxlist.FilterMapList[*catalog.Variant, *preparedResponse](variants, func(_ int, variant *catalog.Variant) (*preparedResponse, bool) {
		response := c.compileVariantResponse(asset, variant)
		return response, response != nil
	})
}

func (c preparedCompiler) compileAssetResponse(asset *catalog.Asset) *preparedResponse {
	result := resolver.Result{
		Asset:     asset,
		FilePath:  asset.FullPath,
		MediaType: asset.MediaType,
		ETag:      asset.ETag,
	}
	return c.compileResponse(result, "")
}

func (c preparedCompiler) compileVariantResponse(asset *catalog.Asset, variant *catalog.Variant) *preparedResponse {
	if !isPreparedVariantUsable(variant, asset.SourceHash) {
		return nil
	}

	result := resolver.Result{
		Asset:   asset,
		Variant: variant,
	}
	result.FilePath = variant.ArtifactPath
	result.MediaType = lo.Ternary(strings.TrimSpace(variant.MediaType) != "", variant.MediaType, asset.MediaType)
	result.ETag = lo.Ternary(strings.TrimSpace(variant.ETag) != "", variant.ETag, asset.ETag)
	if variant.Encoding != "" {
		result.ContentEncoding = variant.Encoding
		result.MediaType = asset.MediaType
	}
	return c.compileResponse(result, variant.Format)
}

func (c preparedCompiler) compileResponse(result resolver.Result, explicitFormat string) *preparedResponse {
	headerPlan := newResolvedHeaderPlan(c.policy, &result, "")
	explicitHeaderPlan := newResolvedHeaderPlan(c.policy, &result, explicitFormat)
	response := &preparedResponse{
		result:             result,
		headerPlan:         newPreparedHeaderPlan(headerPlan),
		explicitHeaderPlan: newPreparedHeaderPlan(explicitHeaderPlan),
	}
	if response.result.Variant != nil {
		response.servedResult = &response.result
	}
	response.resourceHints = c.compileResourceHints(&result)
	response.resourceHintHeader = resourceHintHeader(response.resourceHints)
	response.body, response.bodyPrepared = c.compileBody(&result)
	return response
}

func (c preparedCompiler) compileResourceHints(result *resolver.Result) *cxlist.List[string] {
	if c.resourceHints == nil {
		return nil
	}
	return c.resourceHints.Links(result)
}

func (c preparedCompiler) compileBody(result *resolver.Result) ([]byte, bool) {
	request := buildMemoryCacheRequest(result, resolver.Request{})
	if c.memoryPolicy == nil || !c.memoryPolicy.ShouldServe(request) {
		return nil, false
	}
	reserved, ok := c.bodyBudget.Reserve(request.Size)
	if !ok {
		return nil, false
	}
	body, err := readServerAssetFileWithGuard(result.FilePath, c.fileGuards)
	if err != nil {
		c.bodyBudget.Release(reserved)
		if c.logger != nil {
			c.logger.Debug("Compile prepared response body failed",
				slog.String("path", result.FilePath),
				slog.Any("error", err),
			)
		}
		return nil, false
	}
	if !c.bodyBudget.Adjust(reserved, int64(len(body))) {
		return nil, false
	}
	return body, true
}

func isPreparedVariantUsable(variant *catalog.Variant, assetSourceHash string) bool {
	if variant == nil || strings.TrimSpace(variant.ArtifactPath) == "" {
		return false
	}
	return assetSourceHash == "" || variant.SourceHash == "" || variant.SourceHash == assetSourceHash
}

func variantImageFormat(response *preparedResponse) string {
	if response == nil || response.result.Variant == nil {
		return ""
	}
	if format := strings.TrimSpace(response.result.Variant.Format); format != "" {
		return format
	}
	return media.ImageFormat(response.result.MediaType)
}

func routePreparedBodyStats(route *preparedRoute) (int, int64) {
	if route == nil {
		return 0, 0
	}
	entries := 0
	bytes := int64(0)
	add := func(response *preparedResponse) {
		if response != nil && response.bodyPrepared {
			entries++
			bytes += int64(len(response.body))
		}
	}
	add(route.identity)
	route.encodings.Range(func(_ string, response *preparedResponse) bool {
		add(response)
		return true
	})
	route.images.Range(func(_ string, responses *cxlist.List[*preparedResponse]) bool {
		responses.Range(func(_ int, response *preparedResponse) bool {
			add(response)
			return true
		})
		return true
	})
	return entries, bytes
}

func routeVariantCount(route *preparedRoute) int {
	if route == nil {
		return 0
	}
	count := route.encodings.Len()
	route.images.Range(func(_ string, responses *cxlist.List[*preparedResponse]) bool {
		count += responses.Len()
		return true
	})
	return count
}

func comparePreparedImageResponses(left, right *preparedResponse) int {
	leftVariant := left.variant()
	rightVariant := right.variant()
	if leftVariant == nil || rightVariant == nil {
		return 0
	}
	return cmp.Or(
		strings.Compare(leftVariant.Format, rightVariant.Format),
		cmp.Compare(leftVariant.Width, rightVariant.Width),
		strings.Compare(leftVariant.ID, rightVariant.ID),
	)
}

func preparedEntryAlias(routePath, entry string) string {
	suffix := "/" + strings.TrimSpace(entry)
	if strings.TrimSpace(routePath) == "" || suffix == "/" || !strings.HasSuffix(routePath, suffix) {
		return ""
	}
	return strings.TrimSuffix(routePath, suffix)
}

func preparedContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return oops.Wrapf(err, "prepared compile context")
	}
	return nil
}

func preparedCompileError(err error) error {
	if err == nil {
		return nil
	}
	return oops.Wrapf(err, "compile prepared snapshot")
}
