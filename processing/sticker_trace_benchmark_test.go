package processing

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
)

var stickerTraceBenchmarkSink int

func BenchmarkStickerTraceTransformPNG(b *testing.B) {
	sourceImage := newBenchmarkStickerTraceImage(1024, 1024)

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, sourceImage); err != nil {
		b.Fatalf("encode source image: %v", err)
	}

	sourceBytes := encoded.Bytes()
	ctx := context.Background()

	b.ReportAllocs()
	b.SetBytes(int64(len(sourceBytes)))
	b.ResetTimer()

	for b.Loop() {
		transformedBytes, err := transformStickerTraceImageForTest(ctx, sourceBytes)
		if err != nil {
			b.Fatalf("transform sticker trace png path: %v", err)
		}

		stickerTraceBenchmarkSink = len(transformedBytes)
	}
}

func BenchmarkStickerTraceTransformNRGBA(b *testing.B) {
	sourceImage := newBenchmarkStickerTraceImage(1024, 1024)
	ctx := context.Background()

	b.ReportAllocs()
	b.SetBytes(int64(len(sourceImage.Pix)))
	b.ResetTimer()

	for b.Loop() {
		transformedPixels, transformed, err := transformStickerTraceNRGBA(
			ctx,
			sourceImage.Pix,
			sourceImage.Bounds().Dx(),
			sourceImage.Bounds().Dy(),
		)
		if err != nil {
			b.Fatalf("transform sticker trace raw nrgba path: %v", err)
		}

		if !transformed {
			b.Fatal("expected benchmark sample image to be transformed")
		}

		stickerTraceBenchmarkSink = len(transformedPixels)
	}
}

func newBenchmarkStickerTraceImage(width int, height int) *image.NRGBA {
	imageData := image.NewNRGBA(image.Rect(0, 0, width, height))

	for y := 120; y < height-120; y++ {
		for x := 140; x < width-140; x++ {
			imageData.SetNRGBA(x, y, color.NRGBA{R: 224, G: 60, B: 54, A: 255})
		}
	}

	for y := 260; y < height-260; y++ {
		for x := 260; x < width-260; x++ {
			imageData.SetNRGBA(x, y, color.NRGBA{})
		}
	}

	for i := range width {
		y := i * height / width
		if y >= 0 && y < height {
			imageData.SetNRGBA(i, y, color.NRGBA{R: 50, G: 160, B: 230, A: 255})
			if y+1 < height {
				imageData.SetNRGBA(i, y+1, color.NRGBA{R: 50, G: 160, B: 230, A: 255})
			}
		}
	}

	for index := range 1400 {
		x := (index * 73) % width
		y := (index * 151) % height
		imageData.SetNRGBA(x, y, color.NRGBA{R: 32, G: 212, B: 120, A: 255})
	}

	return imageData
}
