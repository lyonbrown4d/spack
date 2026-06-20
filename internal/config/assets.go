// Package config loads and exposes runtime configuration.
package config

// FallbackOn defines the condition under which a fallback file is served.
type FallbackOn string

const (
	// FallbackOnNotFound indicates that the fallback target
	// will be served when the requested asset is not found
	// in the asset registry (i.e. lookup miss).
	FallbackOnNotFound FallbackOn = "not_found"

	// FallbackOnForbidden indicates that the fallback target
	// will be served when the requested asset exists but is
	// not accessible due to permission or policy restrictions.
	FallbackOnForbidden FallbackOn = "forbidden"
)

// Assets defines the static asset serving configuration.
// It represents a single mount point that maps an HTTP URL path
// prefix to a filesystem root directory or a .spack bundle.
//
// The design assumes:
//   - one process serves one asset mount
//   - no dynamic routing or multi-location resolution
//   - all assets are scanned and registered at startup
type Assets struct {
	// Path is the HTTP request path prefix used to match incoming requests.
	//
	// It must start with '/' and represents the virtual root of static assets.
	// For example:
	//   "/"      → serve assets from the site root
	//   "/static" → serve assets under /static/*
	//
	// This path is matched before any filesystem lookup occurs.
	Path string `koanf:"path" validate:"required,startswith=/"`

	// Root is the filesystem directory or .spack bundle used as the source of static assets.
	//
	// All files under this directory or bundle will be scanned at startup
	// and registered into the in-memory asset registry.
	//
	// This path should point to an existing directory and is
	// typically resolved to an absolute path during initialization.
	Root string `koanf:"root" validate:"required"`

	// Entry is the default entry file name used for directory requests.
	//
	// When a request resolves to a directory path, the server will
	// attempt to serve the file specified by Entry within that directory.
	//
	// Typical values include:
	//   - "index.html"
	//
	// Entry must be a relative file name and must not start with '/'.
	Entry string `koanf:"entry" validate:"required,spack_relative_path"`

	// Fallback defines the fallback serving behavior when a request
	// cannot be resolved normally.
	//
	// The fallback target must already exist in the asset registry
	// and is resolved as a virtual path (not a filesystem path).
	//
	// If no fallback behavior is desired, this field should be left
	// empty in the configuration.
	Fallback Fallback `koanf:"fallback" validate:"required"`
}

// Fallback defines the rules for serving a fallback asset
// when a request cannot be fulfilled under certain conditions.
type Fallback struct {
	// On specifies the condition that triggers the fallback.
	//
	// Supported values:
	//   - "not_found"  : triggered when asset lookup fails
	//   - "forbidden"  : triggered when asset access is denied
	On FallbackOn `koanf:"on" validate:"omitempty,oneof=not_found forbidden"`

	// Target is the virtual asset path to be served as fallback.
	//
	// This path is resolved against the asset registry and must
	// exist at startup time. It typically points to an entry file
	// such as "/index.html".
	//
	// The target is not re-scanned or dynamically resolved at runtime.
	Target string `koanf:"target" validate:"required_with=On,omitempty,spack_relative_path"`
}
