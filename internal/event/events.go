package event

import "time"

const (
	variantServedEventName    = "variant.served"
	variantGeneratedEventName = "variant.generated"
	variantRemovedEventName   = "variant.removed"
	catalogChangedEventName   = "catalog.changed"
)

type VariantRemovalReason string

const (
	VariantRemovalReasonTTL  VariantRemovalReason = "ttl"
	VariantRemovalReasonSize VariantRemovalReason = "size"
)

type VariantServed struct {
	AssetPath     string
	ArtifactPath  string
	ServedAt      time.Time
	ContentType   string
	ContentCoding string
}

func (VariantServed) Name() string {
	return variantServedEventName
}

type VariantGenerated struct {
	AssetPath    string
	ArtifactPath string
	Stage        string
	Size         int64
	GeneratedAt  time.Time
}

func (VariantGenerated) Name() string {
	return variantGeneratedEventName
}

type VariantRemoved struct {
	ArtifactPath string
	Reason       VariantRemovalReason
	RemovedAt    time.Time
}

func (VariantRemoved) Name() string {
	return variantRemovedEventName
}

type CatalogChanged struct {
	Reason    string
	ChangedAt time.Time
}

func (CatalogChanged) Name() string {
	return catalogChangedEventName
}
