package processing

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestTransformBlurhashImageKeepsTargetDimensions(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 16, 12))

	for y := range 12 {
		for x := range 16 {
			source.Set(x, y, color.NRGBA{R: uint8(x * 12), G: uint8(y * 16), B: 180, A: 255})
		}
	}

	var sourceBuffer bytes.Buffer
	if err := png.Encode(&sourceBuffer, source); err != nil {
		t.Fatalf("encode source png: %v", err)
	}

	transformedBytes, err := transformBlurhashImage(context.Background(), sourceBuffer.Bytes(), blurhashImageTransformOptions{
		TargetWidth:  120,
		TargetHeight: 80,
		XComponents:  4,
		YComponents:  3,
		Punch:        1,
		ResolutionX:  32,
		ResolutionY:  32,
	})
	if err != nil {
		t.Fatalf("transform blurhash image: %v", err)
	}

	decodedImage, _, err := image.Decode(bytes.NewReader(transformedBytes))
	if err != nil {
		t.Fatalf("decode transformed image: %v", err)
	}

	if decodedImage.Bounds().Dx() != 120 || decodedImage.Bounds().Dy() != 80 {
		t.Fatalf("expected transformed image size 120x80, got %dx%d", decodedImage.Bounds().Dx(), decodedImage.Bounds().Dy())
	}
}

func TestTransformBlurhashImageRespectsCanceledContext(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	source.Set(0, 0, color.NRGBA{R: 255, G: 100, B: 40, A: 255})

	var sourceBuffer bytes.Buffer
	if err := png.Encode(&sourceBuffer, source); err != nil {
		t.Fatalf("encode source png: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := transformBlurhashImage(ctx, sourceBuffer.Bytes(), blurhashImageTransformOptions{
		TargetWidth:  128,
		TargetHeight: 128,
		XComponents:  4,
		YComponents:  3,
		Punch:        1,
		ResolutionX:  32,
		ResolutionY:  32,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}
