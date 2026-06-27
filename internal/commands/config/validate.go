package configcmd

import (
	"github.com/lyonbrown4d/spack/internal/source"
	"github.com/samber/oops"
)

func validateConfiguredAssetsRoot(root string) error {
	return newValidateUseCase(source.NewResolver()).Validate(root)
}

type validateUseCase struct {
	resolver *source.Resolver
}

func newValidateUseCase(resolver *source.Resolver) validateUseCase {
	if resolver == nil {
		resolver = source.NewResolver()
	}
	return validateUseCase{resolver: resolver}
}

func (u validateUseCase) Validate(root string) error {
	_, err := u.resolver.Resolve(root)
	return oops.Wrapf(err, "validate assets root")
}
