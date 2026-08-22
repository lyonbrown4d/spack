// Package media centralizes media-type and image-format normalization helpers.
package media

import (
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/lyonbrown4d/spack/pkg"
)

type ImageFormatDescriptor struct {
	Name         string
	MediaType    string
	Extension    string
	AcceptTokens *cxlist.List[string]
}

var (
	imageFormatDescriptors = cxlist.NewList[ImageFormatDescriptor](
		ImageFormatDescriptor{
			Name:         "jpeg",
			MediaType:    "image/jpeg",
			Extension:    ".jpg",
			AcceptTokens: cxlist.NewList[string]("image/jpeg", "image/jpg"),
		},
		ImageFormatDescriptor{
			Name:         "png",
			MediaType:    "image/png",
			Extension:    ".png",
			AcceptTokens: cxlist.NewList[string]("image/png"),
		},
		ImageFormatDescriptor{
			Name:         "webp",
			MediaType:    "image/webp",
			Extension:    ".webp",
			AcceptTokens: cxlist.NewList[string]("image/webp"),
		},
		ImageFormatDescriptor{
			Name:         "avif",
			MediaType:    "image/avif",
			Extension:    ".avif",
			AcceptTokens: cxlist.NewList[string]("image/avif"),
		},
	)
	imageDescriptorsByName = cxmapping.AssociateList(
		imageFormatDescriptors,
		func(_ int, descriptor ImageFormatDescriptor) (string, ImageFormatDescriptor) {
			return descriptor.Name, descriptor
		},
	)
	imageDescriptorsByMediaType = cxmapping.AssociateList(
		imageFormatDescriptors,
		func(_ int, descriptor ImageFormatDescriptor) (string, ImageFormatDescriptor) {
			return descriptor.MediaType, descriptor
		},
	)
	imageDescriptorsByAcceptToken = buildImageDescriptorsByAcceptToken(imageFormatDescriptors)
)

func SupportedImageFormats() *cxlist.List[string] {
	return cxlist.MapList(imageFormatDescriptors, func(_ int, descriptor ImageFormatDescriptor) string {
		return descriptor.Name
	})
}

func LookupImageDescriptor(format string) (ImageFormatDescriptor, bool) {
	return imageDescriptorsByName.Get(pkg.TrimLower(format))
}

func LookupImageDescriptorByMediaType(mediaType string) (ImageFormatDescriptor, bool) {
	return imageDescriptorsByMediaType.Get(pkg.TrimLower(mediaType))
}

func LookupImageDescriptorByAcceptToken(token string) (ImageFormatDescriptor, bool) {
	return imageDescriptorsByAcceptToken.Get(pkg.TrimLower(token))
}

func NormalizeImageFormat(format string) string {
	normalized := pkg.TrimLower(format)
	switch normalized {
	case "jpg":
		return "jpeg"
	default:
		if descriptor, ok := LookupImageDescriptor(normalized); ok {
			return descriptor.Name
		}
		return ""
	}
}

func ImageFormat(mediaType string) string {
	if descriptor, ok := LookupImageDescriptorByMediaType(mediaType); ok {
		return descriptor.Name
	}
	return ""
}

func IsImageMediaType(mediaType string) bool {
	return strings.HasPrefix(pkg.TrimLower(mediaType), "image/")
}

func NormalizeImageFormats(formats *cxlist.List[string]) *cxlist.List[string] {
	if formats == nil || formats.IsEmpty() {
		return nil
	}
	return pkg.NormalizeStringList(formats, NormalizeImageFormat, pkg.PreserveOrder)
}

func buildImageDescriptorsByAcceptToken(
	descriptors *cxlist.List[ImageFormatDescriptor],
) *cxmapping.Map[string, ImageFormatDescriptor] {
	out := cxmapping.NewMap[string, ImageFormatDescriptor]()
	descriptors.Range(func(_ int, descriptor ImageFormatDescriptor) bool {
		descriptor.AcceptTokens.Range(func(_ int, token string) bool {
			normalized := pkg.TrimLower(token)
			if normalized != "" {
				out.Set(normalized, descriptor)
			}
			return true
		})
		return true
	})
	return out
}
