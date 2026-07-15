// Package main generates deterministic static assets for the k6 Docker benchmark.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

const (
	smallTextSize = 4 * 1024
	appJSSize     = 128 * 1024
	vendorJSSize  = 512 * 1024
	styleCSSSize  = 64 * 1024
	fontSize      = 96 * 1024
	largeBinSize  = 1024 * 1024
)

func main() {
	out := flag.String("out", filepath.Join("tmp", "k6", "assets"), "output directory")
	goarch := flag.String("goarch", "amd64", "linux GOARCH directory to prepare for Docker build context")
	flag.Parse()

	root := filepath.Dir(*out)
	if err := os.MkdirAll(*out, 0o750); err != nil {
		fail("create output directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "results"), 0o750); err != nil {
		fail("create results directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "linux", *goarch), 0o750); err != nil {
		fail("create linux/%s directory: %v", *goarch, err)
	}
	writeAsset(*out, "index.html", frontendHTML())
	writeAsset(*out, "small.txt", repeatedPayload([]byte("spack k6 small text payload 0123456789\n"), smallTextSize))
	writeAsset(*out, "app.js", repeatedPayload([]byte("export const payload = 'spack k6 javascript benchmark payload';\n"), appJSSize))
	writeAsset(*out, "large.bin", repeatedPayload(binaryBlock(), largeBinSize))
	writeAsset(*out, "manifest.webmanifest", []byte(`{"name":"SPACK k6 bench","short_name":"SPACK","start_url":"/","display":"standalone","icons":[]}`+"\n"))
	writeAsset(*out, filepath.Join("assets", "app.8f3a1c2d.js"), repeatedPayload([]byte(appJSBlock()), appJSSize))
	writeAsset(*out, filepath.Join("assets", "vendor.6d1e3a9b.js"), repeatedPayload([]byte(vendorJSBlock()), vendorJSSize))
	writeAsset(*out, filepath.Join("assets", "style.4b21c9aa.css"), repeatedPayload([]byte(styleCSSBlock()), styleCSSSize))
	writeAsset(*out, filepath.Join("assets", "logo.1a2b3c4d.svg"), []byte(logoSVG()))
	writeAsset(*out, filepath.Join("assets", "font.7f6e5d4c.woff2"), repeatedPayload(binaryBlock(), fontSize))
	writeJPEG(*out, filepath.Join("assets", "hero.9d7f2a10.jpg"), 1920, 1080)
	writePNG(*out, filepath.Join("assets", "card.2e4c91ab.png"), 900, 600)
}

func writeAsset(root, name string, payload []byte) {
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fail("create asset directory for %s: %v", name, err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		fail("write %s: %v", name, err)
	}
}

func writeJPEG(root, name string, width, height int) {
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, gradientImage(width, height), &jpeg.Options{Quality: 82}); err != nil {
		fail("encode %s: %v", name, err)
	}
	writeAsset(root, name, buffer.Bytes())
}

func writePNG(root, name string, width, height int) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, checkerImage(width, height)); err != nil {
		fail("encode %s: %v", name, err)
	}
	writeAsset(root, name, buffer.Bytes())
}

func repeatedPayload(block []byte, size int) []byte {
	repeated := bytes.Repeat(block, size/len(block)+1)
	return repeated[:size]
}

func frontendHTML() []byte {
	return []byte(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>SPACK k6 frontend bench</title>
  <link rel="manifest" href="/manifest.webmanifest">
  <link rel="preload" href="/assets/font.7f6e5d4c.woff2" as="font" type="font/woff2" crossorigin>
  <link rel="stylesheet" href="/assets/style.4b21c9aa.css">
  <link rel="modulepreload" href="/assets/vendor.6d1e3a9b.js">
  <script type="module" src="/assets/app.8f3a1c2d.js"></script>
</head>
<body>
  <main class="shell">
    <img class="logo" src="/assets/logo.1a2b3c4d.svg" alt="SPACK">
    <img class="hero" src="/assets/hero.9d7f2a10.jpg" alt="Generated benchmark hero">
    <img class="card" src="/assets/card.2e4c91ab.png" alt="Generated benchmark card">
    <h1>Static frontend benchmark</h1>
  </main>
</body>
</html>
`)
}

func appJSBlock() string {
	return strings.Join([]string{
		"import './vendor.6d1e3a9b.js';",
		"const root = document.querySelector('.shell');",
		"const values = Array.from({ length: 128 }, (_, index) => index * 13);",
		"root?.setAttribute('data-bench', values.reduce((sum, value) => sum + value, 0).toString());",
		"export const spackBenchApp = values;",
	}, "\n") + "\n"
}

func vendorJSBlock() string {
	return strings.Join([]string{
		"export function hydrateBenchNode(node) {",
		"  if (!node) return;",
		"  node.dataset.vendorReady = 'true';",
		"}",
		"export const vendorPayload = new Map(Array.from({ length: 512 }, (_, index) => [index, `chunk-${index}`]));",
	}, "\n") + "\n"
}

func styleCSSBlock() string {
	return strings.Join([]string{
		":root { --bg: #f6f2e9; --fg: #18201b; --accent: #c86f2f; }",
		"@font-face { font-family: BenchSans; src: url('/assets/font.7f6e5d4c.woff2') format('woff2'); font-display: swap; }",
		"body { margin: 0; min-height: 100vh; color: var(--fg); background: linear-gradient(135deg, #f6f2e9, #dce8df); font-family: BenchSans, serif; }",
		".shell { width: min(1120px, calc(100vw - 48px)); margin: 0 auto; padding: 48px 0; }",
		".hero { width: 100%; border-radius: 28px; box-shadow: 0 32px 80px rgba(24,32,27,.18); }",
		".card { width: min(420px, 80vw); margin-top: 24px; border-radius: 20px; }",
		".logo { width: 120px; display: block; margin-bottom: 24px; }",
	}, "\n") + "\n"
}

func logoSVG() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 80" role="img">
  <rect width="240" height="80" rx="22" fill="#18201b"/>
  <path d="M34 52 58 20l24 32H66l-8-11-8 11H34Z" fill="#f6f2e9"/>
  <text x="96" y="51" fill="#f6f2e9" font-family="Georgia,serif" font-size="32" font-weight="700">SPACK</text>
</svg>
`
}

func gradientImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var green uint8
	for y := range height {
		var red uint8
		for x := range width {
			img.SetRGBA(x, y, color.RGBA{
				R: red,
				G: green,
				B: red ^ green,
				A: 255,
			})
			red += 3
		}
		green += 2
	}
	return img
}

func checkerImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	palette := []color.RGBA{
		{R: 24, G: 32, B: 27, A: 255},
		{R: 200, G: 111, B: 47, A: 255},
		{R: 246, G: 242, B: 233, A: 255},
	}
	for y := range height {
		for x := range width {
			index := ((x / 24) + (y / 24)) % len(palette)
			img.SetRGBA(x, y, palette[index])
		}
	}
	return img
}

func binaryBlock() []byte {
	block := make([]byte, 256)
	for i := range block {
		block[i] = byte(i)
	}
	return block
}

func fail(format string, args ...any) {
	if _, err := fmt.Fprintf(os.Stderr, format+"\n", args...); err != nil {
		os.Exit(1)
	}
	os.Exit(1)
}
