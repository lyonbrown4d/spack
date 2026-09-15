package config

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/KimMachineGun/automemlimit/memlimit"
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

const (
	memoryCacheFallbackBytes = 64 * 1024 * 1024
	memoryCacheMinimumBytes  = 4 * 1024 * 1024
	memoryCacheMaximumBytes  = 256 * 1024 * 1024
	memoryCacheBudgetDivisor = 16
)

func defaultMemoryCacheMaxBytes() int64 {
	if limit := debug.SetMemoryLimit(-1); limit < math.MaxInt64 {
		return memoryCacheMaxBytes(limit)
	}

	limit, err := memlimit.ApplyFallback(memlimit.FromCgroup, memlimit.FromSystem)()
	if err != nil || limit > math.MaxInt64 {
		return memoryCacheFallbackBytes
	}

	return memoryCacheMaxBytes(int64(limit))
}

func memoryCacheMaxBytes(memoryLimit int64) int64 {
	if memoryLimit <= 0 {
		return memoryCacheFallbackBytes
	}

	return min(max(memoryLimit/memoryCacheBudgetDivisor, memoryCacheMinimumBytes), memoryCacheMaximumBytes)
}
func defaultHTTPConfig() HTTP {
	return HTTP{
		Port:                80,
		LowMemory:           false,
		ExposeServerHeader:  false,
		ExposeServerVersion: false,
		MemoryCache: MemoryCache{
			Enable:      true,
			Warmup:      true,
			MaxEntries:  1024,
			MaxBytes:    defaultMemoryCacheMaxBytes(),
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
		Level: "info",
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
		Enable:      false,
		PprofPrefix: "",
	}
}

func defaultImageConfig() Image {
	return Image{
		Enable:               true,
		Widths:               "640,1280,1920",
		Formats:              "",
		JPEGQuality:          78,
		MaxSourceBytes:       10 * 1024 * 1024,
		MaxSourcePixels:      25_000_000,
		MaxWidth:             10_000,
		MaxHeight:            10_000,
		MaxOutputVariants:    12,
		MaxConcurrentSources: 2,
		MaxMemoryBytes:       128 * 1024 * 1024,
		MinSavingRatio:       0.05,
		MinSavingBytes:       1024,
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
		StaleAssetRecovery: StaleAssetRecovery{
			Enable: false,
		},
	}
}

func defaultCompressionConfig() Compression {
	return Compression{
		Mode:                  CompressionModeWarmup,
		Enable:                true,
		CacheDir:              filepath.Join(os.TempDir(), "spack-cache"),
		MinSize:               1024,
		Workers:               2,
		QueueSize:             0,
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
