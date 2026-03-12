package processing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"
)

const (
	stickerTraceMaxSourceImagePixels    = 16_777_216
	stickerTraceMaxSourceImageDimension = 8192

	stickerTraceAlphaThreshold = 4

	stickerTraceFlatPixelCheckStride = 4096
	stickerTraceRowCheckStride       = 8
	stickerTraceQueueCheckStride     = 1024
	stickerTraceDistanceCheckStride  = 256

	stickerTraceMinBorderPixels = 6
	stickerTraceMaxBorderPixels = 60
	stickerTraceBorderRatio     = 0.04

	stickerTraceGapClosingRatio     = 0.08
	stickerTraceMinGapClosingPixels = 1
	stickerTraceMaxGapClosingPixels = 4

	stickerTraceMinComponentPixels = 18
	stickerTraceComponentRatio     = 0.00012
)

type bounds struct {
	minX int
	minY int
	maxX int
	maxY int
}

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

	sourceMask, err := buildVisibleMask(ctx, sourceData, width, height)
	if err != nil {
		return nil, false, err
	}

	sourceBounds, ok, err := getMaskBounds(ctx, sourceMask, width, height)
	if err != nil {
		return nil, false, err
	}

	if !ok {
		return nil, false, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	borderPixels := getBorderSize(width, height)
	scaleFactor := getSafeScaleFactor(sourceBounds, width, height, borderPixels)
	scaledData, err := scaleAndCenterImage(ctx, sourceData, width, height, sourceBounds, scaleFactor)
	if err != nil {
		return nil, false, err
	}

	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	scaledVisibleMask, err := buildVisibleMask(ctx, scaledData, width, height)
	if err != nil {
		return nil, false, err
	}

	cleanedVisibleMask, err := removeTinyDetachedComponents(ctx, scaledVisibleMask, width, height)
	if err != nil {
		return nil, false, err
	}

	stickerBaseMask, err := fillStickerTraceGaps(ctx, cleanedVisibleMask, width, height, borderPixels)
	if err != nil {
		return nil, false, err
	}

	stickerDistanceField, err := buildDistanceField(ctx, stickerBaseMask, width, height)
	if err != nil {
		return nil, false, err
	}

	stickerCoreMask, err := buildStickerCoreMask(ctx, stickerDistanceField, borderPixels)
	if err != nil {
		return nil, false, err
	}

	stickerFillMask, err := fillStickerTraceGaps(ctx, stickerCoreMask, width, height, borderPixels)
	if err != nil {
		return nil, false, err
	}

	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	outputData, err := composeStickerImage(
		ctx,
		scaledData,
		cleanedVisibleMask,
		stickerDistanceField,
		stickerFillMask,
		borderPixels,
	)
	if err != nil {
		return nil, false, err
	}

	return outputData, true, nil
}

func getBorderSize(width int, height int) int {
	borderByRatio := int(math.Round(float64(min(width, height)) * stickerTraceBorderRatio))
	return max(stickerTraceMinBorderPixels, min(stickerTraceMaxBorderPixels, borderByRatio))
}

func buildVisibleMask(ctx context.Context, imageData []uint8, width int, height int) ([]uint8, error) {
	mask := make([]uint8, width*height)
	err := parallelForContext(ctx, len(mask), func(start int, end int) error {
		for index := start; index < end; index++ {
			if err := checkContextInterval(ctx, index, stickerTraceFlatPixelCheckStride); err != nil {
				return err
			}

			if imageData[index*4+3] > stickerTraceAlphaThreshold {
				mask[index] = 1
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return mask, nil
}

func getMaskBounds(ctx context.Context, mask []uint8, width int, height int) (bounds, bool, error) {
	minX := width
	minY := height
	maxX := -1
	maxY := -1

	for y := range height {
		if err := checkContextInterval(ctx, y, stickerTraceRowCheckStride); err != nil {
			return bounds{}, false, err
		}

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
		return bounds{}, false, nil
	}

	return bounds{minX: minX, minY: minY, maxX: maxX, maxY: maxY}, true, nil
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

func scaleAndCenterImage(
	ctx context.Context,
	sourceData []uint8,
	width int,
	height int,
	maskBounds bounds,
	scaleFactor float64,
) ([]uint8, error) {
	outputData := make([]uint8, width*height*4)
	sourceCenterX := float64(maskBounds.minX+maskBounds.maxX) / 2
	sourceCenterY := float64(maskBounds.minY+maskBounds.maxY) / 2
	outputCenterX := float64(width-1) / 2
	outputCenterY := float64(height-1) / 2

	err := parallelForContext(ctx, height, func(start int, end int) error {
		for y := start; y < end; y++ {
			if err := checkContextInterval(ctx, y, stickerTraceRowCheckStride); err != nil {
				return err
			}

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

		return nil
	})
	if err != nil {
		return nil, err
	}

	return outputData, nil
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

func fillInternalHoles(ctx context.Context, mask []uint8, width int, height int) ([]uint8, error) {
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
		if err := checkContextInterval(ctx, queueStart, stickerTraceQueueCheckStride); err != nil {
			return nil, err
		}

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
		if err := checkContextInterval(ctx, index, stickerTraceFlatPixelCheckStride); err != nil {
			return nil, err
		}

		if mask[index] == 1 || visitedOutside[index] == 0 {
			filledMask[index] = 1
		}
	}

	return filledMask, nil
}

func fillStickerTraceGaps(ctx context.Context, mask []uint8, width int, height int, borderPixels int) ([]uint8, error) {
	gapClosingPixels := getGapClosingPixels(borderPixels)
	closedMask, err := closeMask(ctx, mask, width, height, gapClosingPixels)
	if err != nil {
		return nil, err
	}

	return fillInternalHoles(ctx, closedMask, width, height)
}

func getGapClosingPixels(borderPixels int) int {
	gapClosingByRatio := int(math.Round(float64(borderPixels) * stickerTraceGapClosingRatio))
	return max(stickerTraceMinGapClosingPixels, min(stickerTraceMaxGapClosingPixels, gapClosingByRatio))
}

func closeMask(ctx context.Context, mask []uint8, width int, height int, radius int) ([]uint8, error) {
	if radius <= 0 {
		return append([]uint8(nil), mask...), nil
	}

	dilatedMask, err := dilateMask(ctx, mask, width, height, radius)
	if err != nil {
		return nil, err
	}

	return erodeMask(ctx, dilatedMask, width, height, radius)
}

func dilateMask(ctx context.Context, mask []uint8, width int, height int, radius int) ([]uint8, error) {
	outputMask := make([]uint8, len(mask))

	err := parallelForContext(ctx, height, func(start int, end int) error {
		for y := start; y < end; y++ {
			if err := checkContextInterval(ctx, y, stickerTraceRowCheckStride); err != nil {
				return err
			}

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

		return nil
	})
	if err != nil {
		return nil, err
	}

	return outputMask, nil
}

func erodeMask(ctx context.Context, mask []uint8, width int, height int, radius int) ([]uint8, error) {
	outputMask := make([]uint8, len(mask))

	err := parallelForContext(ctx, height, func(start int, end int) error {
		for y := start; y < end; y++ {
			if err := checkContextInterval(ctx, y, stickerTraceRowCheckStride); err != nil {
				return err
			}

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

		return nil
	})
	if err != nil {
		return nil, err
	}

	return outputMask, nil
}

func removeTinyDetachedComponents(ctx context.Context, mask []uint8, width int, height int) ([]uint8, error) {
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
		if err := checkContextInterval(ctx, index, stickerTraceFlatPixelCheckStride); err != nil {
			return nil, err
		}

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
			if err := checkContextInterval(ctx, queueStart, stickerTraceQueueCheckStride); err != nil {
				return nil, err
			}

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
		if err := checkContextInterval(ctx, index, stickerTraceFlatPixelCheckStride); err != nil {
			return nil, err
		}

		label := labels[index]
		if label == 0 {
			continue
		}

		componentSize := componentSizes[label]
		if label == largestComponentLabel || componentSize >= minimumComponentPixels {
			cleanedMask[index] = 1
		}
	}

	return cleanedMask, nil
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

func buildDistanceField(ctx context.Context, mask []uint8, width int, height int) ([]float64, error) {
	const infinity = 1e20
	columnPass := make([]float64, len(mask))

	err := parallelForContext(ctx, width, func(start int, end int) error {
		for x := start; x < end; x++ {
			if err := checkContextInterval(ctx, x, stickerTraceRowCheckStride); err != nil {
				return err
			}

			column := make([]float64, height)
			for y := range height {
				if mask[y*width+x] == 1 {
					column[y] = 0
				} else {
					column[y] = infinity
				}
			}

			transformedColumn, err := squaredDistanceTransform1d(ctx, column)
			if err != nil {
				return err
			}
			for y := range height {
				columnPass[y*width+x] = transformedColumn[y]
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	fullPass := make([]float64, len(mask))
	err = parallelForContext(ctx, height, func(start int, end int) error {
		for y := start; y < end; y++ {
			if err := checkContextInterval(ctx, y, stickerTraceRowCheckStride); err != nil {
				return err
			}

			row := make([]float64, width)
			for x := range width {
				row[x] = columnPass[y*width+x]
			}

			transformedRow, err := squaredDistanceTransform1d(ctx, row)
			if err != nil {
				return err
			}
			for x := range width {
				fullPass[y*width+x] = transformedRow[x]
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return fullPass, nil
}

func buildStickerCoreMask(ctx context.Context, stickerDistanceField []float64, stickerRadius int) ([]uint8, error) {
	coreDistanceSquared := float64(stickerRadius * stickerRadius)
	coreMask := make([]uint8, len(stickerDistanceField))

	err := parallelForContext(ctx, len(stickerDistanceField), func(start int, end int) error {
		for index := start; index < end; index++ {
			if err := checkContextInterval(ctx, index, stickerTraceFlatPixelCheckStride); err != nil {
				return err
			}

			if stickerDistanceField[index] <= coreDistanceSquared {
				coreMask[index] = 1
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return coreMask, nil
}

func squaredDistanceTransform1d(ctx context.Context, values []float64) ([]float64, error) {
	length := len(values)
	locations := make([]int, length)
	boundaries := make([]float64, length+1)
	output := make([]float64, length)
	hullSize := 0

	locations[0] = 0
	boundaries[0] = math.Inf(-1)
	boundaries[1] = math.Inf(1)

	for query := 1; query < length; query++ {
		if err := checkContextInterval(ctx, query, stickerTraceDistanceCheckStride); err != nil {
			return nil, err
		}

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
		if err := checkContextInterval(ctx, query, stickerTraceDistanceCheckStride); err != nil {
			return nil, err
		}

		for boundaries[hullSize+1] < float64(query) {
			hullSize++
		}

		site := locations[hullSize]
		distance := query - site
		output[query] = float64(distance*distance) + values[site]
	}

	return output, nil
}

func composeStickerImage(
	ctx context.Context,
	imageData []uint8,
	visibleMask []uint8,
	stickerDistanceField []float64,
	stickerFillMask []uint8,
	stickerRadius int,
) ([]uint8, error) {
	outputData := make([]uint8, len(imageData))

	err := parallelForContext(ctx, len(imageData)/4, func(start int, end int) error {
		for index := start; index < end; index++ {
			if err := checkContextInterval(ctx, index, stickerTraceFlatPixelCheckStride); err != nil {
				return err
			}

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

		return nil
	})
	if err != nil {
		return nil, err
	}

	return outputData, nil
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

func parallelForContext(ctx context.Context, total int, run func(start int, end int) error) error {
	if total <= 0 {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	workerCount := runtime.GOMAXPROCS(0)
	if workerCount <= 1 || total < 16384 {
		return run(0, total)
	}

	if workerCount > total {
		workerCount = total
	}

	chunkSize := (total + workerCount - 1) / workerCount
	var waitGroup sync.WaitGroup
	var once sync.Once
	var firstErr error

	for start := 0; start < total; start += chunkSize {
		end := min(total, start+chunkSize)
		waitGroup.Add(1)
		go func(from int, to int) {
			defer waitGroup.Done()

			if err := run(from, to); err != nil {
				once.Do(func() {
					firstErr = err
				})
			}
		}(start, end)
	}

	waitGroup.Wait()

	return firstErr
}

func checkContextInterval(ctx context.Context, index int, interval int) error {
	if interval <= 0 {
		return nil
	}

	if index%interval == 0 {
		return ctx.Err()
	}

	return nil
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
