package config

import (
	"os"
	"path/filepath"
	"runtime"
)

func defaultConfig() Config {
	return Config{
		HTTP: HTTP{
			Port:      80,
			LowMemory: true,
			Prefork:   false,
			MemoryCache: MemoryCache{
				Enable:      true,
				Warmup:      true,
				MaxEntries:  1024,
				MaxBytes:    64 * 1024 * 1024,
				MaxFileSize: 64 * 1024,
				TTL:         "5m",
			},
			RequestLogDetail: false,
		},
		Assets: Assets{
			Backend:  SourceBackendLocal,
			Path:     "/",
			Entry:    "index.html",
			Fallback: Fallback{On: FallbackOnNotFound, Target: "index.html"},
		},
		Async: Async{
			Workers: max(runtime.NumCPU(), 1),
		},
		Logger: Logger{
			Level: "debug",
			Console: Console{
				Enabled: true,
			},
			File: File{Enabled: false},
		},
		Metrics: Metrics{Prefix: "/prometheus"},
		Robots: Robots{
			Enable:    true,
			Override:  false,
			UserAgent: "*",
			Allow:     "/",
		},
		Debug: Debug{
			Enable:      true,
			PprofPrefix: "/pprof",
			LivePort:    8080,
			Address:     "0.0.0.0",
		},
		Image: Image{
			Enable:      true,
			Widths:      "640,1280,1920",
			Formats:     "",
			JPEGQuality: 78,
		},
		Frontend: Frontend{
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
		},
		Compression: Compression{
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
		},
	}
}

func DefaultConfig() Config {
	return defaultConfig()
}
