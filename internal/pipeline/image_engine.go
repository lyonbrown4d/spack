package pipeline

import (
	"context"

	cxlist "github.com/arcgolabs/collectionx/list"
)

type imageEncodeOptions struct {
	JPEGQuality int
}

type imageGenerateLimits struct {
	MaxSourceBytes  int64
	MaxSourcePixels int64
	MaxWidth        int
	MaxHeight       int
	MaxMemoryBytes  int64
}

type imageVariantGenerateRequest struct {
	TargetFormat string
	TargetWidth  int
}

type imageGenerateRequest struct {
	Context         context.Context
	SourcePath      string
	SourceBytes     int64
	SourceMediaType string
	TargetFormat    string
	TargetWidth     int
	Encode          imageEncodeOptions
	Limits          imageGenerateLimits
}

type imageGenerateBatchRequest struct {
	Context         context.Context
	SourcePath      string
	SourceBytes     int64
	SourceMediaType string
	Variants        *cxlist.List[imageVariantGenerateRequest]
	Encode          imageEncodeOptions
	Limits          imageGenerateLimits
}

type imageGenerateResult struct {
	Payload      []byte
	Width        int
	Height       int
	SourceWidth  int
	SourceHeight int
	SourceBytes  int64
	TargetFormat string
	MediaType    string
	Extension    string
}

type imageEngine interface {
	Name() string
	SupportsSourceMediaType(mediaType string) bool
	SupportedTargetFormats() *cxlist.List[string]
	Generate(request imageGenerateRequest) (imageGenerateResult, error)
	GenerateBatch(request imageGenerateBatchRequest) (*cxlist.List[imageGenerateResult], error)
}
