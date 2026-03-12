package processing

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
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

	for y := range 5 {
		mask[y*width+4] = 0
	}

	withoutGapClosing, err := fillInternalHoles(context.Background(), mask, width, height)
	if err != nil {
		t.Fatalf("fill internal holes: %v", err)
	}
	if withoutGapClosing[4*width+4] != 0 {
		t.Fatal("expected channel-connected pixel to remain unfilled without gap closing")
	}

	withGapClosing, err := fillStickerTraceGaps(context.Background(), mask, width, height, 12)
	if err != nil {
		t.Fatalf("fill sticker trace gaps: %v", err)
	}
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

	for y := range 7 {
		for x := 5; x <= 7; x++ {
			mask[y*width+x] = 0
		}
	}

	withGapClosing, err := fillStickerTraceGaps(context.Background(), mask, width, height, 6)
	if err != nil {
		t.Fatalf("fill sticker trace gaps: %v", err)
	}
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
	transformedBytes, err := transformStickerTraceImageForTest(context.Background(), originalBytes)
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

	_, err := transformStickerTraceImageForTest(ctx, imageBuffer.Bytes())
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestTransformStickerTraceNRGBAReturnsUnchangedWhenFullyTransparent(t *testing.T) {
	transparentImage := image.NewNRGBA(image.Rect(0, 0, 4, 4))

	transformedPixels, transformed, err := transformStickerTraceNRGBA(
		context.Background(),
		transparentImage.Pix,
		transparentImage.Bounds().Dx(),
		transparentImage.Bounds().Dy(),
	)
	if err != nil {
		t.Fatalf("transform sticker trace nrgba: %v", err)
	}

	if transformed {
		t.Fatal("expected transparent image to skip raw nrgba sticker trace")
	}

	if transformedPixels != nil {
		t.Fatal("expected transformed pixels to be nil when sticker trace is skipped")
	}
}

func TestTransformStickerTraceNRGBARespectsCanceledContextDuringTransform(t *testing.T) {
	sourceImage := newBenchmarkStickerTraceImage(128, 128)
	ctx := newCancelAfterErrContext(context.Background(), 6)

	_, _, err := transformStickerTraceNRGBA(
		ctx,
		sourceImage.Pix,
		sourceImage.Bounds().Dx(),
		sourceImage.Bounds().Dy(),
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestTransformStickerTraceNRGBAMatchesPNGPath(t *testing.T) {
	sourceImage := image.NewNRGBA(image.Rect(0, 0, 96, 96))

	for y := 16; y < 80; y++ {
		for x := 16; x < 80; x++ {
			sourceImage.SetNRGBA(x, y, color.NRGBA{R: 226, G: 47, B: 47, A: 255})
		}
	}

	for y := 28; y < 68; y++ {
		for x := 28; x < 68; x++ {
			sourceImage.SetNRGBA(x, y, color.NRGBA{})
		}
	}

	for i := range 96 {
		sourceImage.SetNRGBA(8+i/2, 8+i, color.NRGBA{R: 47, G: 150, B: 226, A: 255})
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, sourceImage); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	pngOutputBytes, err := transformStickerTraceImageForTest(context.Background(), encoded.Bytes())
	if err != nil {
		t.Fatalf("transform sticker trace png path: %v", err)
	}

	decodedOutput, err := decodeTestNRGBA(pngOutputBytes)
	if err != nil {
		t.Fatalf("decode transformed png output: %v", err)
	}

	rawOutputPixels, transformed, err := transformStickerTraceNRGBA(
		context.Background(),
		sourceImage.Pix,
		sourceImage.Bounds().Dx(),
		sourceImage.Bounds().Dy(),
	)
	if err != nil {
		t.Fatalf("transform sticker trace raw nrgba path: %v", err)
	}

	if !transformed {
		t.Fatal("expected raw nrgba sticker trace path to transform this test image")
	}

	if !bytes.Equal(rawOutputPixels, decodedOutput.Pix) {
		t.Fatal("expected raw nrgba sticker trace output to match png wrapper output")
	}
}

func decodeTestNRGBA(imageBytes []byte) (*image.NRGBA, error) {
	decodedImage, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, err
	}

	bounds := decodedImage.Bounds()
	output := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(output, output.Bounds(), decodedImage, bounds.Min, draw.Src)

	return output, nil
}
