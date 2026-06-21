package configcmd

func ValidateConfiguredAssetsRootForTest(root string) error {
	return validateConfiguredAssetsRoot(root)
}

func EffectiveSourceInfoForTest(root string, redact bool) (map[string]any, error) {
	return effectiveSourceInfo(root, redact)
}
