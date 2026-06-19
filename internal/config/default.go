package config

import (
	"os"
	"path/filepath"
	"runtime"
)

func defaultConfig() Config {
	return Config{
		APIVersion:  "spack.io/v1alpha1",
		Kind:        "RuntimeConfig",
		HTTP:        defaultHTTPConfig(),
		Assets:      defaultAssetsConfig(),
		Async:       defaultAsyncConfig(),
		Logger:      defaultLoggerConfig(),
		Metrics:     Metrics{Enable: true, Prefix: "/prometheus"},
		Robots:      defaultRobotsConfig(),
		Debug:       defaultDebugConfig(),
		Image:       defaultImageConfig(),
		Frontend:    defaultFrontendConfig(),
		Compression: defaultCompressionConfig(),
	}
}

func defaultHTTPConfig() HTTP {
	return HTTP{
		Port:                80,
		LowMemory:           true,
		ExposeServerHeader:  false,
		ExposeServerVersion: false,
		MemoryCache: MemoryCache{
			Enable:      true,
			Warmup:      true,
			MaxEntries:  1024,
			MaxBytes:    64 * 1024 * 1024,
			MaxFileSize: 64 * 1024,
			TTL:         "5m",
		},
		RequestLogDetail: false,
	}
}

func defaultAssetsConfig() Assets {
	return Assets{
		Path:     "/",
		Entry:    "index.html",
		Fallback: Fallback{On: FallbackOnNotFound, Target: "index.html"},
	}
}

func defaultAsyncConfig() Async {
	return Async{
		Workers: max(runtime.NumCPU(), 1),
	}
}

func defaultLoggerConfig() Logger {
	return Logger{
		Level: "debug",
		Console: Console{
			Enabled: true,
		},
		File: File{Enabled: false},
	}
}

func defaultRobotsConfig() Robots {
	return Robots{
		Enable:    true,
		Override:  false,
		UserAgent: "*",
		Allow:     "/",
	}
}

func defaultDebugConfig() Debug {
	return Debug{
		Enable:      true,
		PprofPrefix: "",
	}
}

func defaultImageConfig() Image {
	return Image{
		Enable:      true,
		Widths:      "640,1280,1920",
		Formats:     "",
		JPEGQuality: 78,
	}
}

func defaultFrontendConfig() Frontend {
	return Frontend{
		ResourceHints: ResourceHints{
			Enable:         true,
			EarlyHints:     false,
			MaxLinks:       16,
			MaxHeaderBytes: 4096,
		},
		ImmutableCache: ImmutableCache{
			Enable: true,
			MaxAge: "8760h",
		},
	}
}

func defaultCompressionConfig() Compression {
	return Compression{
		Mode:                  CompressionModeLazy,
		Enable:                true,
		CacheDir:              filepath.Join(os.TempDir(), "spack-cache"),
		MinSize:               1024,
		Workers:               2,
		QueueSize:             128,
		Encodings:             "br,zstd,gzip",
		CleanupEvery:          "5m",
		MaxAge:                "168h",
		ImageMaxAge:           "336h",
		EncodingMaxAge:        "168h",
		MaxCacheBytes:         1073741824,
		EncodingMaxCacheBytes: 0,
		ImageMaxCacheBytes:    0,
		BrotliQuality:         5,
		ZstdLevel:             3,
		GzipLevel:             5,
	}
}

func DefaultConfig() Config {
	return defaultConfig()
}
