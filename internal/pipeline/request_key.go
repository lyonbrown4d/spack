package pipeline

import (
	cxlist "github.com/arcgolabs/collectionx/list"
	"strconv"
	"strings"
)

func buildRequestKey(assetPath string, encodings, formats *cxlist.List[string], widths *cxlist.List[int]) string {
	var builder strings.Builder
	if !writeBuilderString(&builder, assetPath) {
		return fallbackRequestKey(assetPath, encodings, formats, widths)
	}
	if !writeBuilderString(&builder, "|e=") ||
		!writeStringList(&builder, encodings) ||
		!writeBuilderString(&builder, "|f=") ||
		!writeStringList(&builder, formats) ||
		!writeBuilderString(&builder, "|w=") ||
		!writeIntList(&builder, widths) {
		return fallbackRequestKey(assetPath, encodings, formats, widths)
	}
	return builder.String()
}

func writeStringList(builder *strings.Builder, values *cxlist.List[string]) bool {
	if values == nil {
		return true
	}
	ok := true
	values.Range(func(index int, value string) bool {
		if index > 0 {
			if !writeBuilderByte(builder, ',') {
				ok = false
				return false
			}
		}
		if !writeBuilderString(builder, value) {
			ok = false
			return false
		}
		return true
	})
	return ok
}

func writeIntList(builder *strings.Builder, values *cxlist.List[int]) bool {
	if values == nil {
		return true
	}
	ok := true
	values.Range(func(index int, value int) bool {
		if index > 0 {
			if !writeBuilderByte(builder, ',') {
				ok = false
				return false
			}
		}
		if !writeBuilderString(builder, strconv.Itoa(value)) {
			ok = false
			return false
		}
		return true
	})
	return ok
}

func writeBuilderString(builder *strings.Builder, value string) bool {
	if _, err := builder.WriteString(value); err != nil {
		return false
	}
	return true
}

func writeBuilderByte(builder *strings.Builder, value byte) bool {
	if err := builder.WriteByte(value); err != nil {
		return false
	}
	return true
}

func fallbackRequestKey(
	assetPath string,
	encodings,
	formats *cxlist.List[string],
	widths *cxlist.List[int],
) string {
	encodedWidths := listIntToString(widths)
	return assetPath +
		"|e=" + listStringToString(encodings, ",") +
		"|f=" + listStringToString(formats, ",") +
		"|w=" + encodedWidths
}

func listStringToString(values *cxlist.List[string], joiner string) string {
	if values == nil || values.IsEmpty() {
		return ""
	}

	serialized := make([]string, 0, values.Len())
	values.Range(func(_ int, value string) bool {
		serialized = append(serialized, value)
		return true
	})
	return strings.Join(serialized, joiner)
}

func listIntToString(values *cxlist.List[int]) string {
	if values == nil || values.IsEmpty() {
		return ""
	}

	serialized := make([]string, 0, values.Len())
	values.Range(func(_ int, value int) bool {
		serialized = append(serialized, strconv.Itoa(value))
		return true
	})
	return strings.Join(serialized, ",")
}
