package validation

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	cxlist "github.com/arcgolabs/collectionx/list"
	"github.com/go-playground/validator/v10"
	"github.com/lyonbrown4d/spack/internal/normalizex"
)

type validationRule struct {
	tag string
	fn  validator.Func
}

func New() (*validator.Validate, error) {
	validate := validator.New(validator.WithRequiredStructEnabled())

	rules := cxlist.NewList(
		validationRule{tag: "spack_duration", fn: validatePositiveDuration},
		validationRule{tag: "spack_flexible_duration", fn: validatePositiveFlexibleDuration},
		validationRule{tag: "spack_relative_path", fn: validateRelativePath},
		validationRule{tag: "spack_widths", fn: validateWidths},
	)

	var registerErr error
	rules.Range(func(_ int, rule validationRule) bool {
		if err := validate.RegisterValidation(rule.tag, rule.fn); err != nil {
			registerErr = fmt.Errorf("register %s validator: %w", rule.tag, err)
			return false
		}
		return true
	})
	if registerErr != nil {
		return nil, registerErr
	}

	return validate, nil
}

func validatePositiveDuration(fl validator.FieldLevel) bool {
	raw := normalizex.Trim(fl.Field().String())
	if raw == "" {
		return true
	}
	d, err := time.ParseDuration(raw)
	return err == nil && d > 0
}

func validatePositiveFlexibleDuration(fl validator.FieldLevel) bool {
	raw := normalizex.Trim(fl.Field().String())
	if raw == "" {
		return true
	}
	return ParseFlexibleDuration(raw) > 0
}

func validateRelativePath(fl validator.FieldLevel) bool {
	return IsRelativePath(fl.Field().String())
}

func validateWidths(fl validator.FieldLevel) bool {
	raw := normalizex.Trim(fl.Field().String())
	if raw == "" {
		return true
	}
	return ParseWidths(raw).Len() > 0
}

func IsRelativePath(raw string) bool {
	raw = normalizex.Trim(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "\\") {
		return false
	}
	return !filepath.IsAbs(raw)
}
