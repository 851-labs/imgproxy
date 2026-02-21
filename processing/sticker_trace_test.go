package processing

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestGetGapClosingPixelsBounds(t *testing.T) {
	if got := getGapClosingPixels(0); got != 1 {
		t.Fatalf("expected min gap closing pixels to be 1, got %d", got)
	}

	if got := getGapClosingPixels(500); got != 4 {
		t.Fatalf("expected max gap closing pixels to be 4, got %d", got)
	}
}

func TestFillStickerTraceGapsClosesNarrowChannel(t *testing.T) {
	const width = 9
	const height = 9

	mask := make([]uint8, width*height)
	for y := 2; y <= 6; y++ {
		for x := 2; x <= 6; x++ {
			mask[y*width+x] = 1
		}
	}

	for y := 0; y <= 4; y++ {
		mask[y*width+4] = 0
	}

	withoutGapClosing := fillInternalHoles(mask, width, height)
	if withoutGapClosing[4*width+4] != 0 {
		t.Fatal("expected channel-connected pixel to remain unfilled without gap closing")
	}

	withGapClosing := fillStickerTraceGaps(mask, width, height, 12)
	if withGapClosing[4*width+4] != 1 {
		t.Fatal("expected narrow channel to be closed and interior to be filled")
	}
}

func TestFillStickerTraceGapsKeepsWideChannelOpen(t *testing.T) {
	const width = 12
	const height = 12

	mask := make([]uint8, width*height)
	for y := 3; y <= 9; y++ {
		for x := 3; x <= 9; x++ {
			mask[y*width+x] = 1
		}
	}

	for y := 0; y <= 6; y++ {
		for x := 5; x <= 7; x++ {
			mask[y*width+x] = 0
		}
	}

	withGapClosing := fillStickerTraceGaps(mask, width, height, 6)
	if withGapClosing[6*width+6] != 0 {
		t.Fatal("expected wide channel to remain open after gap closing")
	}
}

func TestTransformStickerTraceImageReturnsOriginalWhenFullyTransparent(t *testing.T) {
	imageBuffer := bytes.Buffer{}
	transparentImage := image.NewNRGBA(image.Rect(0, 0, 4, 4))

	if err := png.Encode(&imageBuffer, transparentImage); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	originalBytes := imageBuffer.Bytes()
	transformedBytes, err := transformStickerTraceImage(context.Background(), originalBytes)
	if err != nil {
		t.Fatalf("transform sticker trace: %v", err)
	}

	if !bytes.Equal(transformedBytes, originalBytes) {
		t.Fatal("expected transparent image to be returned unchanged")
	}
}

func TestTransformStickerTraceImageRespectsCanceledContext(t *testing.T) {
	imageBuffer := bytes.Buffer{}
	nonTransparentImage := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	nonTransparentImage.Set(0, 0, color.NRGBA{R: 255, A: 255})

	if err := png.Encode(&imageBuffer, nonTransparentImage); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := transformStickerTraceImage(ctx, imageBuffer.Bytes())
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
