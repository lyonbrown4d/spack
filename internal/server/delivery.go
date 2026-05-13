package server

import "github.com/gofiber/fiber/v3"

const assetDeliveryLocalKey = "asset_delivery"

const (
	deliveryHotResponseHit  = "hot_response_hit"
	deliveryMemoryCacheHit  = "memory_cache_hit"
	deliveryMemoryCacheFill = "memory_cache_fill"
	deliverySendFile        = "sendfile"
	deliverySendFileRange   = "sendfile_range"
)

func setAssetDelivery(c fiber.Ctx, delivery string) {
	c.Locals(assetDeliveryLocalKey, delivery)
}

func getAssetDelivery(c fiber.Ctx) string {
	value := c.Locals(assetDeliveryLocalKey)
	delivery, ok := value.(string)
	if !ok {
		return ""
	}
	return delivery
}
