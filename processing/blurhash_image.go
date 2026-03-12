package processing

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"

	blurhash "github.com/bbrks/go-blurhash"
	"github.com/imgproxy/imgproxy/v3/imagedata"
	"github.com/imgproxy/imgproxy/v3/imagetype"
	"github.com/imgproxy/imgproxy/v3/options"
)

type blurhashImageTransformOptions struct {
	TargetWidth  int
	TargetHeight int

	XComponents int
	YComponents int
	Punch       float64

	ResolutionX int
	ResolutionY int
}

func (p *Processor) blurhashImage(c *Context) error {
	if !c.PO.BlurhashImageEnabled() {
		return nil
	}

	if c.Img.IsAnimated() {
		return nil
	}

	targetWidth := c.Img.Width()
	targetHeight := c.Img.Height()

	sourceImageData, err := c.Img.Save(imagetype.PNG, 100, options.New())
	if err != nil {
		return err
	}
	defer sourceImageData.Close()

	if err := c.Ctx.Err(); err != nil {
		return err
	}

	sourceBytes, err := io.ReadAll(sourceImageData.Reader())
	if err != nil {
		return err
	}

	if err := c.Ctx.Err(); err != nil {
		return err
	}

	transformedBytes, err := transformBlurhashImage(c.Ctx, sourceBytes, blurhashImageTransformOptions{
		TargetWidth:  targetWidth,
		TargetHeight: targetHeight,
		XComponents:  c.PO.BlurhashImageXComponents(),
		YComponents:  c.PO.BlurhashImageYComponents(),
		Punch:        c.PO.BlurhashImagePunch(),
		ResolutionX:  c.PO.BlurhashImageResolutionX(),
		ResolutionY:  c.PO.BlurhashImageResolutionY(),
	})
	if err != nil {
		return err
	}

	transformedImageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, transformedBytes)
	return c.Img.Load(transformedImageData, 1.0, 0, 1)
}

func transformBlurhashImage(
	ctx context.Context,
	sourceBytes []byte,
	transformOptions blurhashImageTransformOptions,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sourceImage, _, err := image.Decode(bytes.NewReader(sourceBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	encodedBlurhash, err := blurhash.Encode(transformOptions.XComponents, transformOptions.YComponents, sourceImage)
	if err != nil {
		return nil, fmt.Errorf("encode blurhash: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	decodedBlurhash := image.NewNRGBA(image.Rect(0, 0, transformOptions.ResolutionX, transformOptions.ResolutionY))
	if err := blurhash.DecodeDraw(decodedBlurhash, encodedBlurhash, transformOptions.Punch); err != nil {
		return nil, fmt.Errorf("decode blurhash: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	scaledImage, err := resizeBlurhashImage(
		ctx,
		decodedBlurhash,
		transformOptions.TargetWidth,
		transformOptions.TargetHeight,
	)
	if err != nil {
		return nil, err
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, scaledImage); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return encoded.Bytes(), nil
}

func resizeBlurhashImage(
	ctx context.Context,
	source *image.NRGBA,
	targetWidth int,
	targetHeight int,
) (*image.NRGBA, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sourceWidth := source.Bounds().Dx()
	sourceHeight := source.Bounds().Dy()

	if sourceWidth == targetWidth && sourceHeight == targetHeight {
		return source, nil
	}

	target := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	for y := range targetHeight {
		if y%16 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}

		sourceY := scaleCoordinate(y, targetHeight, sourceHeight)
		y0 := int(math.Floor(sourceY))
		y1 := min(y0+1, sourceHeight-1)
		ty := sourceY - float64(y0)

		for x := range targetWidth {
			sourceX := scaleCoordinate(x, targetWidth, sourceWidth)
			x0 := int(math.Floor(sourceX))
			x1 := min(x0+1, sourceWidth-1)
			tx := sourceX - float64(x0)

			targetOffset := y*target.Stride + x*4

			for channel := range 4 {
				c00 := float64(source.Pix[y0*source.Stride+x0*4+channel])
				c10 := float64(source.Pix[y0*source.Stride+x1*4+channel])
				c01 := float64(source.Pix[y1*source.Stride+x0*4+channel])
				c11 := float64(source.Pix[y1*source.Stride+x1*4+channel])

				top := c00*(1.0-tx) + c10*tx
				bottom := c01*(1.0-tx) + c11*tx
				value := top*(1.0-ty) + bottom*ty

				target.Pix[targetOffset+channel] = uint8(math.Round(value))
			}
		}
	}

	return target, nil
}

func scaleCoordinate(position int, dstSize int, srcSize int) float64 {
	if dstSize <= 1 || srcSize <= 1 {
		return 0
	}

	return float64(position) * float64(srcSize-1) / float64(dstSize-1)
}
