package processing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"runtime"
	"slices"
	"sync"
)

const (
	stickerTraceMaxSourceImagePixels    = 16_777_216
	stickerTraceMaxSourceImageDimension = 8192

	stickerTraceAlphaThreshold = 4

	stickerTraceMinBorderPixels = 6
	stickerTraceMaxBorderPixels = 60
	stickerTraceBorderRatio     = 0.04

	stickerTraceGapClosingRatio     = 0.08
	stickerTraceMinGapClosingPixels = 1
	stickerTraceMaxGapClosingPixels = 4

	stickerTraceMinComponentPixels = 18
	stickerTraceComponentRatio     = 0.00012

	stickerTraceColorSpeckleMaxPixels        = 12
	stickerTraceColorSpeckleDistance         = 46
	stickerTraceColorSimilarNeighborDistance = 24

	stickerTraceMinOpaqueAlpha = 220

	stickerTraceOutlierWindowRadius     = 2
	stickerTraceOutlierMinNeighbors     = 12
	stickerTraceOutlierPixelDistance    = 34
	stickerTraceOutlierNeighborSpread   = 26
	stickerTraceOutlierSimilarNeighbors = 8
)

type bounds struct {
	minX int
	minY int
	maxX int
	maxY int
}

type color3 [3]uint8

func (p *Processor) stickerTrace(c *Context) error {
	if !c.PO.StickerTrace() {
		return nil
	}

	sourceData, width, height, err := c.Img.ExportNRGBA()
	if err != nil {
		return err
	}

	transformedData, transformed, err := transformStickerTraceNRGBA(c.Ctx, sourceData, width, height)
	if err != nil {
		return err
	}

	if !transformed {
		return nil
	}

	return c.Img.LoadNRGBA(transformedData, width, height)
}

func transformStickerTraceImage(ctx context.Context, fileBytes []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sourceImage, err := decodeStickerTraceImage(fileBytes)
	if err != nil {
		return nil, err
	}

	width := sourceImage.Bounds().Dx()
	height := sourceImage.Bounds().Dy()
	if width <= 0 || height <= 0 {
		return nil, errors.New("could not read image dimensions")
	}

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

func transformStickerTraceNRGBA(
	ctx context.Context,
	sourceData []uint8,
	width int,
	height int,
) ([]uint8, bool, error) {
	if width <= 0 || height <= 0 {
		return nil, false, errors.New("could not read image dimensions")
	}

	if width > stickerTraceMaxSourceImageDimension || height > stickerTraceMaxSourceImageDimension {
		return nil, false, fmt.Errorf(
			"source image dimensions exceed %dx%d",
			stickerTraceMaxSourceImageDimension,
			stickerTraceMaxSourceImageDimension,
		)
	}

	if width > stickerTraceMaxSourceImagePixels/height {
		return nil, false, fmt.Errorf("source image exceeds %d pixels", stickerTraceMaxSourceImagePixels)
	}

	pixelCount := width * height
	if pixelCount > stickerTraceMaxSourceImagePixels {
		return nil, false, fmt.Errorf("source image exceeds %d pixels", stickerTraceMaxSourceImagePixels)
	}

	expectedSourceDataSize := width * height * 4
	if len(sourceData) != expectedSourceDataSize {
		return nil, false, fmt.Errorf(
			"raw nrgba buffer size %d does not match expected dimensions size %d",
			len(sourceData),
			expectedSourceDataSize,
		)
	}

	sourceMask := buildVisibleMask(sourceData, width, height)
	sourceBounds, ok := getMaskBounds(sourceMask, width, height)
	if !ok {
		return nil, false, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	borderPixels := getBorderSize(width, height)
	scaleFactor := getSafeScaleFactor(sourceBounds, width, height, borderPixels)
	scaledData := scaleAndCenterImage(sourceData, width, height, sourceBounds, scaleFactor)

	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	scaledVisibleMask := buildVisibleMask(scaledData, width, height)
	cleanedVisibleMask := removeTinyDetachedComponents(scaledVisibleMask, width, height)
	cleanedColorData := removeColorSpeckles(scaledData, cleanedVisibleMask, width, height)
	denoisedColorData := removeLocalColorOutliers(cleanedColorData, cleanedVisibleMask, width, height)
	stickerBaseMask := fillStickerTraceGaps(cleanedVisibleMask, width, height, borderPixels)
	stickerDistanceField := buildDistanceField(stickerBaseMask, width, height)
	stickerCoreMask := buildStickerCoreMask(stickerDistanceField, borderPixels)
	stickerFillMask := fillStickerTraceGaps(stickerCoreMask, width, height, borderPixels)

	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	outputData := composeStickerImage(
		denoisedColorData,
		cleanedVisibleMask,
		stickerDistanceField,
		stickerFillMask,
		borderPixels,
	)

	return outputData, true, nil
}

func decodeStickerTraceImage(fileBytes []byte) (*image.NRGBA, error) {
	decoded, _, err := image.Decode(bytes.NewReader(fileBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	bounds := decoded.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid source image dimensions")
	}

	if width > stickerTraceMaxSourceImageDimension || height > stickerTraceMaxSourceImageDimension {
		return nil, fmt.Errorf(
			"source image dimensions exceed %dx%d",
			stickerTraceMaxSourceImageDimension,
			stickerTraceMaxSourceImageDimension,
		)
	}

	if width > stickerTraceMaxSourceImagePixels/height {
		return nil, fmt.Errorf("source image exceeds %d pixels", stickerTraceMaxSourceImagePixels)
	}

	pixelCount := width * height
	if pixelCount > stickerTraceMaxSourceImagePixels {
		return nil, fmt.Errorf("source image exceeds %d pixels", stickerTraceMaxSourceImagePixels)
	}

	output := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(output, output.Bounds(), decoded, bounds.Min, draw.Src)

	return output, nil
}

func getBorderSize(width int, height int) int {
	borderByRatio := int(math.Round(float64(min(width, height)) * stickerTraceBorderRatio))
	return max(stickerTraceMinBorderPixels, min(stickerTraceMaxBorderPixels, borderByRatio))
}

func buildVisibleMask(imageData []uint8, width int, height int) []uint8 {
	mask := make([]uint8, width*height)
	parallelFor(len(mask), func(start int, end int) {
		for index := start; index < end; index++ {
			if imageData[index*4+3] > stickerTraceAlphaThreshold {
				mask[index] = 1
			}
		}
	})
	return mask
}

func getMaskBounds(mask []uint8, width int, height int) (bounds, bool) {
	minX := width
	minY := height
	maxX := -1
	maxY := -1

	for y := range height {
		for x := range width {
			if mask[y*width+x] == 0 {
				continue
			}

			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}

	if maxX < 0 || maxY < 0 {
		return bounds{}, false
	}

	return bounds{minX: minX, minY: minY, maxX: maxX, maxY: maxY}, true
}

func getSafeScaleFactor(maskBounds bounds, width int, height int, borderPixels int) float64 {
	boundsWidth := maskBounds.maxX - maskBounds.minX + 1
	boundsHeight := maskBounds.maxY - maskBounds.minY + 1
	safeCanvasWidth := max(1, width-borderPixels*2)
	safeCanvasHeight := max(1, height-borderPixels*2)
	horizontalScale := float64(safeCanvasWidth) / float64(boundsWidth)
	verticalScale := float64(safeCanvasHeight) / float64(boundsHeight)
	return minFloat(1, horizontalScale, verticalScale)
}

func scaleAndCenterImage(sourceData []uint8, width int, height int, maskBounds bounds, scaleFactor float64) []uint8 {
	outputData := make([]uint8, width*height*4)
	sourceCenterX := float64(maskBounds.minX+maskBounds.maxX) / 2
	sourceCenterY := float64(maskBounds.minY+maskBounds.maxY) / 2
	outputCenterX := float64(width-1) / 2
	outputCenterY := float64(height-1) / 2

	parallelFor(height, func(start int, end int) {
		for y := start; y < end; y++ {
			for x := range width {
				sourceX := (float64(x)-outputCenterX)/scaleFactor + sourceCenterX
				sourceY := (float64(y)-outputCenterY)/scaleFactor + sourceCenterY
				red, green, blue, alpha := sampleBilinear(sourceData, width, height, sourceX, sourceY)
				outputIndex := (y*width + x) * 4
				outputData[outputIndex] = red
				outputData[outputIndex+1] = green
				outputData[outputIndex+2] = blue
				outputData[outputIndex+3] = alpha
			}
		}
	})

	return outputData
}

func sampleBilinear(imageData []uint8, width int, height int, x float64, y float64) (uint8, uint8, uint8, uint8) {
	if x < 0 || y < 0 || x > float64(width-1) || y > float64(height-1) {
		return 0, 0, 0, 0
	}

	x0 := int(math.Floor(x))
	y0 := int(math.Floor(y))
	x1 := min(width-1, x0+1)
	y1 := min(height-1, y0+1)
	tx := x - float64(x0)
	ty := y - float64(y0)

	topLeftIndex := (y0*width + x0) * 4
	topRightIndex := (y0*width + x1) * 4
	bottomLeftIndex := (y1*width + x0) * 4
	bottomRightIndex := (y1*width + x1) * 4

	topLeftR := imageData[topLeftIndex]
	topLeftG := imageData[topLeftIndex+1]
	topLeftB := imageData[topLeftIndex+2]
	topLeftA := imageData[topLeftIndex+3]

	topRightR := imageData[topRightIndex]
	topRightG := imageData[topRightIndex+1]
	topRightB := imageData[topRightIndex+2]
	topRightA := imageData[topRightIndex+3]

	bottomLeftR := imageData[bottomLeftIndex]
	bottomLeftG := imageData[bottomLeftIndex+1]
	bottomLeftB := imageData[bottomLeftIndex+2]
	bottomLeftA := imageData[bottomLeftIndex+3]

	bottomRightR := imageData[bottomRightIndex]
	bottomRightG := imageData[bottomRightIndex+1]
	bottomRightB := imageData[bottomRightIndex+2]
	bottomRightA := imageData[bottomRightIndex+3]

	alpha := bilinearInterpolate(
		float64(topLeftA),
		float64(topRightA),
		float64(bottomLeftA),
		float64(bottomRightA),
		tx,
		ty,
	)

	if alpha <= 0 {
		return 0, 0, 0, 0
	}

	topLeftAlpha := float64(topLeftA) / 255
	topRightAlpha := float64(topRightA) / 255
	bottomLeftAlpha := float64(bottomLeftA) / 255
	bottomRightAlpha := float64(bottomRightA) / 255

	redPremult := bilinearInterpolate(
		float64(topLeftR)*topLeftAlpha,
		float64(topRightR)*topRightAlpha,
		float64(bottomLeftR)*bottomLeftAlpha,
		float64(bottomRightR)*bottomRightAlpha,
		tx,
		ty,
	)
	greenPremult := bilinearInterpolate(
		float64(topLeftG)*topLeftAlpha,
		float64(topRightG)*topRightAlpha,
		float64(bottomLeftG)*bottomLeftAlpha,
		float64(bottomRightG)*bottomRightAlpha,
		tx,
		ty,
	)
	bluePremult := bilinearInterpolate(
		float64(topLeftB)*topLeftAlpha,
		float64(topRightB)*topRightAlpha,
		float64(bottomLeftB)*bottomLeftAlpha,
		float64(bottomRightB)*bottomRightAlpha,
		tx,
		ty,
	)

	alphaNormalized := alpha / 255
	red := clampColor(redPremult / alphaNormalized)
	green := clampColor(greenPremult / alphaNormalized)
	blue := clampColor(bluePremult / alphaNormalized)

	return red, green, blue, uint8(math.Round(alpha))
}

func bilinearInterpolate(
	topLeft float64,
	topRight float64,
	bottomLeft float64,
	bottomRight float64,
	tx float64,
	ty float64,
) float64 {
	top := topLeft + (topRight-topLeft)*tx
	bottom := bottomLeft + (bottomRight-bottomLeft)*tx
	return top + (bottom-top)*ty
}

func fillInternalHoles(mask []uint8, width int, height int) []uint8 {
	visitedOutside := make([]uint8, len(mask))
	queue := make([]int, len(mask))
	queueStart := 0
	queueEnd := 0

	tryQueue := func(x int, y int) {
		if x < 0 || y < 0 || x >= width || y >= height {
			return
		}

		pixelIndex := y*width + x
		if mask[pixelIndex] != 0 || visitedOutside[pixelIndex] == 1 {
			return
		}

		visitedOutside[pixelIndex] = 1
		queue[queueEnd] = pixelIndex
		queueEnd++
	}

	for x := range width {
		tryQueue(x, 0)
		tryQueue(x, height-1)
	}

	for y := range height {
		tryQueue(0, y)
		tryQueue(width-1, y)
	}

	for queueStart < queueEnd {
		pixelIndex := queue[queueStart]
		queueStart++

		x := pixelIndex % width
		y := pixelIndex / width

		tryQueue(x-1, y)
		tryQueue(x+1, y)
		tryQueue(x, y-1)
		tryQueue(x, y+1)
	}

	filledMask := make([]uint8, len(mask))
	for index := range mask {
		if mask[index] == 1 || visitedOutside[index] == 0 {
			filledMask[index] = 1
		}
	}

	return filledMask
}

func fillStickerTraceGaps(mask []uint8, width int, height int, borderPixels int) []uint8 {
	gapClosingPixels := getGapClosingPixels(borderPixels)
	closedMask := closeMask(mask, width, height, gapClosingPixels)
	return fillInternalHoles(closedMask, width, height)
}

func getGapClosingPixels(borderPixels int) int {
	gapClosingByRatio := int(math.Round(float64(borderPixels) * stickerTraceGapClosingRatio))
	return max(stickerTraceMinGapClosingPixels, min(stickerTraceMaxGapClosingPixels, gapClosingByRatio))
}

func closeMask(mask []uint8, width int, height int, radius int) []uint8 {
	if radius <= 0 {
		return append([]uint8(nil), mask...)
	}

	dilatedMask := dilateMask(mask, width, height, radius)
	return erodeMask(dilatedMask, width, height, radius)
}

func dilateMask(mask []uint8, width int, height int, radius int) []uint8 {
	outputMask := make([]uint8, len(mask))

	parallelFor(height, func(start int, end int) {
		for y := start; y < end; y++ {
			for x := range width {
				index := y*width + x
				if mask[index] == 1 {
					outputMask[index] = 1
					continue
				}

				for offsetY := -radius; offsetY <= radius; offsetY++ {
					ny := y + offsetY
					if ny < 0 || ny >= height {
						continue
					}

					foundNeighbor := false
					for offsetX := -radius; offsetX <= radius; offsetX++ {
						nx := x + offsetX
						if nx < 0 || nx >= width {
							continue
						}

						if mask[ny*width+nx] == 1 {
							outputMask[index] = 1
							foundNeighbor = true
							break
						}
					}

					if foundNeighbor {
						break
					}
				}
			}
		}
	})

	return outputMask
}

func erodeMask(mask []uint8, width int, height int, radius int) []uint8 {
	outputMask := make([]uint8, len(mask))

	parallelFor(height, func(start int, end int) {
		for y := start; y < end; y++ {
			for x := range width {
				index := y*width + x
				if mask[index] == 0 {
					continue
				}

				fullyCovered := true
				for offsetY := -radius; offsetY <= radius; offsetY++ {
					ny := y + offsetY
					if ny < 0 || ny >= height {
						fullyCovered = false
						break
					}

					for offsetX := -radius; offsetX <= radius; offsetX++ {
						nx := x + offsetX
						if nx < 0 || nx >= width {
							fullyCovered = false
							break
						}

						if mask[ny*width+nx] == 0 {
							fullyCovered = false
							break
						}
					}

					if !fullyCovered {
						break
					}
				}

				if fullyCovered {
					outputMask[index] = 1
				}
			}
		}
	})

	return outputMask
}

func removeTinyDetachedComponents(mask []uint8, width int, height int) []uint8 {
	labels := make([]int, len(mask))
	queue := make([]int, len(mask))
	componentSizes := []int{0}
	minimumComponentPixels := max(
		stickerTraceMinComponentPixels,
		int(math.Round(float64(width*height)*stickerTraceComponentRatio)),
	)
	nextLabel := 1
	largestComponentLabel := 0

	for index := range mask {
		if mask[index] == 0 || labels[index] != 0 {
			continue
		}

		queueStart := 0
		queueEnd := 0
		componentSize := 0
		queue[queueEnd] = index
		queueEnd++
		labels[index] = nextLabel

		for queueStart < queueEnd {
			pixelIndex := queue[queueStart]
			queueStart++
			componentSize++

			x := pixelIndex % width
			y := pixelIndex / width

			if queueMaskNeighbor(mask, labels, queue, width, height, x-1, y, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueMaskNeighbor(mask, labels, queue, width, height, x+1, y, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueMaskNeighbor(mask, labels, queue, width, height, x, y-1, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueMaskNeighbor(mask, labels, queue, width, height, x, y+1, nextLabel, queueEnd) {
				queueEnd++
			}
		}

		for len(componentSizes) <= nextLabel {
			componentSizes = append(componentSizes, 0)
		}
		componentSizes[nextLabel] = componentSize
		if componentSize > componentSizes[largestComponentLabel] {
			largestComponentLabel = nextLabel
		}
		nextLabel++
	}

	cleanedMask := make([]uint8, len(mask))
	for index := range mask {
		label := labels[index]
		if label == 0 {
			continue
		}

		componentSize := componentSizes[label]
		if label == largestComponentLabel || componentSize >= minimumComponentPixels {
			cleanedMask[index] = 1
		}
	}

	return cleanedMask
}

func queueMaskNeighbor(
	mask []uint8,
	labels []int,
	queue []int,
	width int,
	height int,
	x int,
	y int,
	label int,
	queueEnd int,
) bool {
	if x < 0 || y < 0 || x >= width || y >= height {
		return false
	}

	pixelIndex := y*width + x
	if mask[pixelIndex] == 0 || labels[pixelIndex] != 0 {
		return false
	}

	labels[pixelIndex] = label
	queue[queueEnd] = pixelIndex
	return true
}

func removeColorSpeckles(imageData []uint8, visibleMask []uint8, width int, height int) []uint8 {
	candidateMask := detectColorSpeckleCandidates(imageData, visibleMask, width, height)
	outputData := append([]uint8(nil), imageData...)
	componentLabels := make([]int, len(candidateMask))
	queue := make([]int, len(candidateMask))
	nextLabel := 1

	for index := range candidateMask {
		if candidateMask[index] == 0 || componentLabels[index] != 0 {
			continue
		}

		queueStart := 0
		queueEnd := 0
		componentPixels := make([]int, 0, stickerTraceColorSpeckleMaxPixels+1)

		queue[queueEnd] = index
		queueEnd++
		componentLabels[index] = nextLabel

		for queueStart < queueEnd {
			pixelIndex := queue[queueStart]
			queueStart++
			componentPixels = append(componentPixels, pixelIndex)

			x := pixelIndex % width
			y := pixelIndex / width

			if queueCandidateNeighbor(candidateMask, componentLabels, queue, width, height, x-1, y, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueCandidateNeighbor(candidateMask, componentLabels, queue, width, height, x+1, y, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueCandidateNeighbor(candidateMask, componentLabels, queue, width, height, x, y-1, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueCandidateNeighbor(candidateMask, componentLabels, queue, width, height, x, y+1, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueCandidateNeighbor(candidateMask, componentLabels, queue, width, height, x-1, y-1, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueCandidateNeighbor(candidateMask, componentLabels, queue, width, height, x+1, y-1, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueCandidateNeighbor(candidateMask, componentLabels, queue, width, height, x-1, y+1, nextLabel, queueEnd) {
				queueEnd++
			}
			if queueCandidateNeighbor(candidateMask, componentLabels, queue, width, height, x+1, y+1, nextLabel, queueEnd) {
				queueEnd++
			}
		}

		if len(componentPixels) <= stickerTraceColorSpeckleMaxPixels {
			inpaintSpeckleComponent(outputData, candidateMask, visibleMask, width, height, componentPixels)
		}

		nextLabel++
	}

	return outputData
}

func removeLocalColorOutliers(imageData []uint8, visibleMask []uint8, width int, height int) []uint8 {
	sourceData := append([]uint8(nil), imageData...)
	outputData := append([]uint8(nil), imageData...)
	similarNeighborDistanceSquared :=
		stickerTraceColorSimilarNeighborDistance * stickerTraceColorSimilarNeighborDistance

	for y := stickerTraceOutlierWindowRadius; y < height-stickerTraceOutlierWindowRadius; y++ {
		for x := stickerTraceOutlierWindowRadius; x < width-stickerTraceOutlierWindowRadius; x++ {
			index := y*width + x
			if visibleMask[index] == 0 {
				continue
			}

			pixelIndex := index * 4
			alpha := getBufferValue(sourceData, pixelIndex+3)
			if alpha < stickerTraceMinOpaqueAlpha {
				continue
			}

			var neighborColorBuffer [24]color3
			neighborColors := getNeighborColorsWithRadius(
				sourceData,
				visibleMask,
				width,
				height,
				x,
				y,
				stickerTraceOutlierWindowRadius,
				neighborColorBuffer[:0],
			)
			if len(neighborColors) < stickerTraceOutlierMinNeighbors {
				continue
			}

			medianColor := getMedianColor(neighborColors)
			neighborDistances := make([]int, 0, len(neighborColors))
			for _, neighborColor := range neighborColors {
				neighborDistances = append(neighborDistances, colorDistanceSquared(neighborColor, medianColor))
			}
			slices.Sort(neighborDistances)

			spreadIndex := int(math.Floor(float64(len(neighborDistances)) * 0.75))
			neighborhoodSpread := 0
			if spreadIndex >= 0 && spreadIndex < len(neighborDistances) {
				neighborhoodSpread = neighborDistances[spreadIndex]
			}
			if neighborhoodSpread > stickerTraceOutlierNeighborSpread*stickerTraceOutlierNeighborSpread {
				continue
			}

			pixelColor := color3{
				getBufferValue(sourceData, pixelIndex),
				getBufferValue(sourceData, pixelIndex+1),
				getBufferValue(sourceData, pixelIndex+2),
			}
			pixelDistanceSquared := colorDistanceSquared(pixelColor, medianColor)
			if pixelDistanceSquared < stickerTraceOutlierPixelDistance*stickerTraceOutlierPixelDistance {
				continue
			}

			similarNeighbors := 0
			for _, neighborColor := range neighborColors {
				if colorDistanceSquared(pixelColor, neighborColor) <= similarNeighborDistanceSquared {
					similarNeighbors++
				}
			}

			if similarNeighbors > stickerTraceOutlierSimilarNeighbors {
				continue
			}

			replacementColor := getNeighborhoodReplacementColor(neighborColors, medianColor)
			outputData[pixelIndex] = replacementColor[0]
			outputData[pixelIndex+1] = replacementColor[1]
			outputData[pixelIndex+2] = replacementColor[2]
		}
	}

	return outputData
}

func detectColorSpeckleCandidates(imageData []uint8, visibleMask []uint8, width int, height int) []uint8 {
	candidateMask := make([]uint8, len(visibleMask))
	similarNeighborDistanceSquared :=
		stickerTraceColorSimilarNeighborDistance * stickerTraceColorSimilarNeighborDistance

	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			index := y*width + x
			if visibleMask[index] == 0 {
				continue
			}

			pixelIndex := index * 4
			alpha := imageData[pixelIndex+3]
			if alpha < stickerTraceMinOpaqueAlpha {
				continue
			}

			var neighborColorBuffer [8]color3
			neighborColors := getNeighborColors(imageData, visibleMask, width, height, x, y, neighborColorBuffer[:0])
			if len(neighborColors) < 5 {
				continue
			}

			medianColor := getMedianColor(neighborColors)
			pixelColor := color3{
				imageData[pixelIndex],
				imageData[pixelIndex+1],
				imageData[pixelIndex+2],
			}
			colorDistance := colorDistanceSquared(pixelColor, medianColor)

			similarNeighbors := 0
			for _, neighborColor := range neighborColors {
				if colorDistanceSquared(pixelColor, neighborColor) <= similarNeighborDistanceSquared {
					similarNeighbors++
				}
			}

			if colorDistance >= stickerTraceColorSpeckleDistance*stickerTraceColorSpeckleDistance && similarNeighbors <= 1 {
				candidateMask[index] = 1
			}
		}
	}

	return candidateMask
}

func queueCandidateNeighbor(
	candidateMask []uint8,
	labels []int,
	queue []int,
	width int,
	height int,
	x int,
	y int,
	label int,
	queueEnd int,
) bool {
	if x < 0 || y < 0 || x >= width || y >= height {
		return false
	}

	pixelIndex := y*width + x
	if candidateMask[pixelIndex] == 0 || labels[pixelIndex] != 0 {
		return false
	}

	labels[pixelIndex] = label
	queue[queueEnd] = pixelIndex
	return true
}

func inpaintSpeckleComponent(
	outputData []uint8,
	candidateMask []uint8,
	visibleMask []uint8,
	width int,
	height int,
	componentPixels []int,
) {
	for _, componentIndex := range componentPixels {
		x := componentIndex % width
		y := componentIndex / width

		redTotal := 0.0
		greenTotal := 0.0
		blueTotal := 0.0
		weightTotal := 0.0

		for offsetY := -2; offsetY <= 2; offsetY++ {
			for offsetX := -2; offsetX <= 2; offsetX++ {
				nx := x + offsetX
				ny := y + offsetY
				if nx < 0 || ny < 0 || nx >= width || ny >= height {
					continue
				}

				neighborIndex := ny*width + nx
				if visibleMask[neighborIndex] == 0 || candidateMask[neighborIndex] == 1 {
					continue
				}

				neighborPixelIndex := neighborIndex * 4
				alpha := getBufferValue(outputData, neighborPixelIndex+3)
				if alpha < stickerTraceMinOpaqueAlpha {
					continue
				}

				distanceSquared := offsetX*offsetX + offsetY*offsetY
				weight := 1 / math.Max(1, float64(distanceSquared))
				redTotal += float64(getBufferValue(outputData, neighborPixelIndex)) * weight
				greenTotal += float64(getBufferValue(outputData, neighborPixelIndex+1)) * weight
				blueTotal += float64(getBufferValue(outputData, neighborPixelIndex+2)) * weight
				weightTotal += weight
			}
		}

		if weightTotal == 0 {
			continue
		}

		pixelIndex := componentIndex * 4
		outputData[pixelIndex] = clampColor(redTotal / weightTotal)
		outputData[pixelIndex+1] = clampColor(greenTotal / weightTotal)
		outputData[pixelIndex+2] = clampColor(blueTotal / weightTotal)
	}
}

func getNeighborColors(
	imageData []uint8,
	visibleMask []uint8,
	width int,
	height int,
	x int,
	y int,
	buffer []color3,
) []color3 {
	colors := buffer[:0]

	for offsetY := -1; offsetY <= 1; offsetY++ {
		for offsetX := -1; offsetX <= 1; offsetX++ {
			if offsetX == 0 && offsetY == 0 {
				continue
			}

			nx := x + offsetX
			ny := y + offsetY
			if nx < 0 || ny < 0 || nx >= width || ny >= height {
				continue
			}

			index := ny*width + nx
			if visibleMask[index] == 0 {
				continue
			}

			pixelIndex := index * 4
			if imageData[pixelIndex+3] < stickerTraceMinOpaqueAlpha {
				continue
			}

			colors = append(colors, color3{
				imageData[pixelIndex],
				imageData[pixelIndex+1],
				imageData[pixelIndex+2],
			})
		}
	}

	return colors
}

func getNeighborColorsWithRadius(
	imageData []uint8,
	visibleMask []uint8,
	width int,
	height int,
	x int,
	y int,
	radius int,
	buffer []color3,
) []color3 {
	colors := buffer[:0]

	for offsetY := -radius; offsetY <= radius; offsetY++ {
		for offsetX := -radius; offsetX <= radius; offsetX++ {
			if offsetX == 0 && offsetY == 0 {
				continue
			}

			nx := x + offsetX
			ny := y + offsetY
			if nx < 0 || ny < 0 || nx >= width || ny >= height {
				continue
			}

			index := ny*width + nx
			if visibleMask[index] == 0 {
				continue
			}

			pixelIndex := index * 4
			if imageData[pixelIndex+3] < stickerTraceMinOpaqueAlpha {
				continue
			}

			colors = append(colors, color3{
				imageData[pixelIndex],
				imageData[pixelIndex+1],
				imageData[pixelIndex+2],
			})
		}
	}

	return colors
}

func getNeighborhoodReplacementColor(neighborColors []color3, medianColor color3) color3 {
	redTotal := 0.0
	greenTotal := 0.0
	blueTotal := 0.0
	weightTotal := 0.0

	for _, neighborColor := range neighborColors {
		distance := math.Sqrt(float64(colorDistanceSquared(neighborColor, medianColor)))
		weight := 1 / (1 + distance)
		redTotal += float64(neighborColor[0]) * weight
		greenTotal += float64(neighborColor[1]) * weight
		blueTotal += float64(neighborColor[2]) * weight
		weightTotal += weight
	}

	if weightTotal <= 0 {
		return medianColor
	}

	return color3{
		clampColor(redTotal / weightTotal),
		clampColor(greenTotal / weightTotal),
		clampColor(blueTotal / weightTotal),
	}
}

func getMedianColor(colors []color3) color3 {
	return color3{
		medianChannel(colors, 0),
		medianChannel(colors, 1),
		medianChannel(colors, 2),
	}
}

func medianChannel(colors []color3, channel int) uint8 {
	var counts [256]int
	for _, color := range colors {
		counts[color[channel]]++
	}

	target := len(colors) / 2
	total := 0
	for value := uint8(0); ; value++ {
		total += counts[value]
		if total > target {
			return value
		}

		if value == math.MaxUint8 {
			break
		}
	}

	return 0
}

func colorDistanceSquared(a color3, b color3) int {
	dr := int(a[0]) - int(b[0])
	dg := int(a[1]) - int(b[1])
	db := int(a[2]) - int(b[2])
	return dr*dr + dg*dg + db*db
}

func getBufferValue(buffer []uint8, index int) uint8 {
	if index < 0 || index >= len(buffer) {
		return 0
	}

	return buffer[index]
}

func buildDistanceField(mask []uint8, width int, height int) []float64 {
	const infinity = 1e20
	columnPass := make([]float64, len(mask))

	parallelFor(width, func(start int, end int) {
		for x := start; x < end; x++ {
			column := make([]float64, height)
			for y := range height {
				if mask[y*width+x] == 1 {
					column[y] = 0
				} else {
					column[y] = infinity
				}
			}

			transformedColumn := squaredDistanceTransform1d(column)
			for y := range height {
				columnPass[y*width+x] = transformedColumn[y]
			}
		}
	})

	fullPass := make([]float64, len(mask))
	parallelFor(height, func(start int, end int) {
		for y := start; y < end; y++ {
			row := make([]float64, width)
			for x := range width {
				row[x] = columnPass[y*width+x]
			}

			transformedRow := squaredDistanceTransform1d(row)
			for x := range width {
				fullPass[y*width+x] = transformedRow[x]
			}
		}
	})

	return fullPass
}

func buildStickerCoreMask(stickerDistanceField []float64, stickerRadius int) []uint8 {
	coreDistanceSquared := float64(stickerRadius * stickerRadius)
	coreMask := make([]uint8, len(stickerDistanceField))

	parallelFor(len(stickerDistanceField), func(start int, end int) {
		for index := start; index < end; index++ {
			if stickerDistanceField[index] <= coreDistanceSquared {
				coreMask[index] = 1
			}
		}
	})

	return coreMask
}

func squaredDistanceTransform1d(values []float64) []float64 {
	length := len(values)
	locations := make([]int, length)
	boundaries := make([]float64, length+1)
	output := make([]float64, length)
	hullSize := 0

	locations[0] = 0
	boundaries[0] = math.Inf(-1)
	boundaries[1] = math.Inf(1)

	for query := 1; query < length; query++ {
		var intersection float64
		for {
			site := locations[hullSize]
			intersection = (values[query] + float64(query*query) - (values[site] + float64(site*site))) / float64((query-site)*2)
			if intersection > boundaries[hullSize] {
				break
			}
			hullSize--
		}

		hullSize++
		locations[hullSize] = query
		boundaries[hullSize] = intersection
		boundaries[hullSize+1] = math.Inf(1)
	}

	hullSize = 0
	for query := range length {
		for boundaries[hullSize+1] < float64(query) {
			hullSize++
		}

		site := locations[hullSize]
		distance := query - site
		output[query] = float64(distance*distance) + values[site]
	}

	return output
}

func composeStickerImage(
	imageData []uint8,
	visibleMask []uint8,
	stickerDistanceField []float64,
	stickerFillMask []uint8,
	stickerRadius int,
) []uint8 {
	outputData := make([]uint8, len(imageData))

	parallelFor(len(imageData)/4, func(start int, end int) {
		for index := start; index < end; index++ {
			pixelIndex := index * 4

			imageAlpha := 0.0
			if visibleMask[index] == 1 {
				imageAlpha = float64(imageData[pixelIndex+3]) / 255
			}

			stickerAlpha := getStickerAlpha(stickerDistanceField[index], stickerRadius)
			if stickerFillMask[index] == 1 {
				stickerAlpha = 1
			}
			outputAlpha := imageAlpha + stickerAlpha*(1-imageAlpha)

			if outputAlpha <= 0 {
				continue
			}

			stickerContribution := stickerAlpha * (1 - imageAlpha)
			red := (float64(imageData[pixelIndex])*imageAlpha + 255*stickerContribution) / outputAlpha
			green := (float64(imageData[pixelIndex+1])*imageAlpha + 255*stickerContribution) / outputAlpha
			blue := (float64(imageData[pixelIndex+2])*imageAlpha + 255*stickerContribution) / outputAlpha

			outputData[pixelIndex] = clampColor(red)
			outputData[pixelIndex+1] = clampColor(green)
			outputData[pixelIndex+2] = clampColor(blue)
			outputData[pixelIndex+3] = clampColor(outputAlpha * 255)
		}
	})

	return outputData
}

func getStickerAlpha(distanceSquared float64, radius int) float64 {
	distance := math.Sqrt(distanceSquared)
	innerEdge := maxFloat(0, float64(radius)-0.5)
	outerEdge := float64(radius) + 0.5

	if distance <= innerEdge {
		return 1
	}

	if distance >= outerEdge {
		return 0
	}

	return (outerEdge - distance) / (outerEdge - innerEdge)
}

func clampColor(value float64) uint8 {
	if value <= 0 {
		return 0
	}
	if value >= 255 {
		return 255
	}
	return uint8(math.Round(value))
}

func parallelFor(total int, run func(start int, end int)) {
	if total <= 0 {
		return
	}

	workerCount := runtime.GOMAXPROCS(0)
	if workerCount <= 1 || total < 16384 {
		run(0, total)
		return
	}

	if workerCount > total {
		workerCount = total
	}

	chunkSize := (total + workerCount - 1) / workerCount
	var waitGroup sync.WaitGroup
	for start := 0; start < total; start += chunkSize {
		end := min(total, start+chunkSize)
		waitGroup.Add(1)
		go func(from int, to int) {
			defer waitGroup.Done()
			run(from, to)
		}(start, end)
	}

	waitGroup.Wait()
}

func minFloat(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}

	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}

	return minimum
}

func maxFloat(a float64, b float64) float64 {
	if a > b {
		return a
	}

	return b
}
