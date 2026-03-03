package processing

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
)

func transformStickerTraceImageForTest(ctx context.Context, fileBytes []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sourceImage, err := decodeStickerTraceImageForTest(fileBytes)
	if err != nil {
		return nil, err
	}

	width := sourceImage.Bounds().Dx()
	height := sourceImage.Bounds().Dy()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	outputData, transformed, err := transformStickerTraceNRGBA(ctx, sourceImage.Pix, width, height)
	if err != nil {
		return nil, err
	}

	if !transformed {
		return fileBytes, nil
	}

	outputImage := &image.NRGBA{
		Pix:    outputData,
		Stride: width * 4,
		Rect:   image.Rect(0, 0, width, height),
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, outputImage); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}

	return encoded.Bytes(), nil
}

func decodeStickerTraceImageForTest(fileBytes []byte) (*image.NRGBA, error) {
	decoded, _, err := image.Decode(bytes.NewReader(fileBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := decoded.Bounds()
	output := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(output, output.Bounds(), decoded, bounds.Min, draw.Src)

	return output, nil
}
