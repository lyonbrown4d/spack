// Package media centralizes media-type and image-format normalization helpers.
package media

import (
	"strings"

	cxlist "github.com/arcgolabs/collectionx/list"
	cxmapping "github.com/arcgolabs/collectionx/mapping"
	cxset "github.com/arcgolabs/collectionx/set"
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
	)
	imageDescriptorsByName = cxmapping.AssociateList[ImageFormatDescriptor, string, ImageFormatDescriptor](
		imageFormatDescriptors,
		func(_ int, descriptor ImageFormatDescriptor) (string, ImageFormatDescriptor) {
			return descriptor.Name, descriptor
		},
	)
	imageDescriptorsByMediaType = cxmapping.AssociateList[ImageFormatDescriptor, string, ImageFormatDescriptor](
		imageFormatDescriptors,
		func(_ int, descriptor ImageFormatDescriptor) (string, ImageFormatDescriptor) {
			return descriptor.MediaType, descriptor
		},
	)
	imageDescriptorsByAcceptToken = buildImageDescriptorsByAcceptToken(imageFormatDescriptors)
)

func SupportedImageFormats() *cxlist.List[string] {
	return cxlist.MapList[ImageFormatDescriptor, string](imageFormatDescriptors, func(_ int, descriptor ImageFormatDescriptor) string {
		return descriptor.Name
	})
}

func LookupImageDescriptor(format string) (ImageFormatDescriptor, bool) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	return imageDescriptorsByName.Get(normalized)
}

func LookupImageDescriptorByMediaType(mediaType string) (ImageFormatDescriptor, bool) {
	normalized := strings.ToLower(strings.TrimSpace(mediaType))
	return imageDescriptorsByMediaType.Get(normalized)
}

func LookupImageDescriptorByAcceptToken(token string) (ImageFormatDescriptor, bool) {
	normalized := strings.ToLower(strings.TrimSpace(token))
	return imageDescriptorsByAcceptToken.Get(normalized)
}

func NormalizeImageFormat(format string) string {
	normalized := strings.ToLower(strings.TrimSpace(format))
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
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mediaType)), "image/")
}

func NormalizeImageFormats(formats *cxlist.List[string]) *cxlist.List[string] {
	if formats == nil || formats.IsEmpty() {
		return nil
	}

	normalized := cxset.NewOrderedSetWithCapacity[string](formats.Len())
	formats.Range(func(_ int, format string) bool {
		value := NormalizeImageFormat(format)
		if value != "" {
			normalized.Add(value)
		}
		return true
	})
	return cxlist.NewList[string](normalized.Values()...)
}

func buildImageDescriptorsByAcceptToken(
	descriptors *cxlist.List[ImageFormatDescriptor],
) *cxmapping.Map[string, ImageFormatDescriptor] {
	out := cxmapping.NewMap[string, ImageFormatDescriptor]()
	descriptors.Range(func(_ int, descriptor ImageFormatDescriptor) bool {
		descriptor.AcceptTokens.Range(func(_ int, token string) bool {
			normalized := strings.ToLower(strings.TrimSpace(token))
			if normalized != "" {
				out.Set(normalized, descriptor)
			}
			return true
		})
		return true
	})
	return out
}
