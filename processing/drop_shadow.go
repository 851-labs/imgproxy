package processing

import "math"

const (
	dropShadowVisibleAlphaCutoff = 0.1
	dropShadowMinimumScale       = 0.9
	dropShadowScaleSearchSteps   = 20
)

type dropShadowMargins struct {
	left   int
	right  int
	top    int
	bottom int
}

func (p *Processor) dropShadow(c *Context) error {
	layers := c.PO.DropShadowLayers()
	if len(layers) == 0 || !c.Img.HasAlpha() {
		return nil
	}

	if err := c.Img.CopyMemory(); err != nil {
		return err
	}

	imageWidth := c.Img.Width()
	imageHeight := c.Img.Height()

	margins, hasAlphaContent, err := dropShadowAlphaMargins(c, imageWidth, imageHeight)
	if err != nil {
		return err
	}

	if !hasAlphaContent {
		return nil
	}

	adjustedLayers := adjustDropShadowLayersForMinimumScale(layers, imageWidth, imageHeight, margins)

	if err := insetImageForDropShadow(c, adjustedLayers, margins); err != nil {
		return err
	}

	xOffsets := make([]int, 0, len(layers))
	yOffsets := make([]int, 0, len(layers))
	blurs := make([]float64, 0, len(layers))
	opacities := make([]float64, 0, len(layers))

	for _, layer := range adjustedLayers {
		xOffsets = append(xOffsets, layer.XOffset)
		yOffsets = append(yOffsets, layer.YOffset)
		blurs = append(blurs, layer.Blur)
		opacities = append(opacities, layer.Opacity)
	}

	return c.Img.ApplyDropShadow(xOffsets, yOffsets, blurs, opacities)
}

func insetImageForDropShadow(c *Context, layers []DropShadowLayer, margins dropShadowMargins) error {
	imageWidth := c.Img.Width()
	imageHeight := c.Img.Height()

	insetLeft, insetRight, insetTop, insetBottom := dropShadowInsetDeficits(layers, margins)

	if insetLeft == 0 && insetRight == 0 && insetTop == 0 && insetBottom == 0 {
		return nil
	}

	targetWidth := max(1, imageWidth-insetLeft-insetRight)
	targetHeight := max(1, imageHeight-insetTop-insetBottom)

	horizontalScale := float64(targetWidth) / float64(imageWidth)
	verticalScale := float64(targetHeight) / float64(imageHeight)
	uniformScale := min(horizontalScale, verticalScale)

	if uniformScale < 1 {
		if err := c.Img.Resize(uniformScale, uniformScale); err != nil {
			return err
		}
	}

	resizedWidth := c.Img.Width()
	resizedHeight := c.Img.Height()

	availableHorizontalInset := imageWidth - resizedWidth
	availableVerticalInset := imageHeight - resizedHeight

	offsetX := resolveInsetOffset(availableHorizontalInset, insetLeft, insetRight)
	offsetY := resolveInsetOffset(availableVerticalInset, insetTop, insetBottom)

	return c.Img.Embed(imageWidth, imageHeight, offsetX, offsetY)
}

func dropShadowAlphaMargins(c *Context, imageWidth int, imageHeight int) (dropShadowMargins, bool, error) {
	alphaLeft, alphaTop, alphaWidth, alphaHeight, err := c.Img.AlphaBounds()
	if err != nil {
		return dropShadowMargins{}, false, err
	}

	if alphaWidth == 0 || alphaHeight == 0 {
		return dropShadowMargins{}, false, nil
	}

	return dropShadowMargins{
		left:   alphaLeft,
		right:  imageWidth - alphaLeft - alphaWidth,
		top:    alphaTop,
		bottom: imageHeight - alphaTop - alphaHeight,
	}, true, nil
}

func adjustDropShadowLayersForMinimumScale(
	layers []DropShadowLayer,
	imageWidth int,
	imageHeight int,
	margins dropShadowMargins,
) []DropShadowLayer {
	if dropShadowMinimumScale <= 0 {
		return layers
	}

	if dropShadowRequiredScale(layers, imageWidth, imageHeight, margins) >= dropShadowMinimumScale {
		return layers
	}

	lowerBound := 0.0
	upperBound := 1.0

	for range dropShadowScaleSearchSteps {
		middle := (lowerBound + upperBound) / 2
		scaledLayers := scaleDropShadowLayers(layers, middle)

		if dropShadowRequiredScale(scaledLayers, imageWidth, imageHeight, margins) >= dropShadowMinimumScale {
			lowerBound = middle
		} else {
			upperBound = middle
		}
	}

	return scaleDropShadowLayers(layers, lowerBound)
}

func dropShadowRequiredScale(
	layers []DropShadowLayer,
	imageWidth int,
	imageHeight int,
	margins dropShadowMargins,
) float64 {
	insetLeft, insetRight, insetTop, insetBottom := dropShadowInsetDeficits(layers, margins)

	targetWidth := max(1, imageWidth-insetLeft-insetRight)
	targetHeight := max(1, imageHeight-insetTop-insetBottom)

	horizontalScale := float64(targetWidth) / float64(imageWidth)
	verticalScale := float64(targetHeight) / float64(imageHeight)

	return min(horizontalScale, verticalScale)
}

func dropShadowInsetDeficits(layers []DropShadowLayer, margins dropShadowMargins) (int, int, int, int) {
	requiredLeft, requiredRight, requiredTop, requiredBottom := dropShadowRequiredInsets(layers)

	insetLeft := max(0, requiredLeft-margins.left)
	insetRight := max(0, requiredRight-margins.right)
	insetTop := max(0, requiredTop-margins.top)
	insetBottom := max(0, requiredBottom-margins.bottom)

	return insetLeft, insetRight, insetTop, insetBottom
}

func scaleDropShadowLayers(layers []DropShadowLayer, scale float64) []DropShadowLayer {
	if scale >= 1 {
		return layers
	}

	scaledLayers := make([]DropShadowLayer, 0, len(layers))

	for _, layer := range layers {
		scaledLayers = append(
			scaledLayers,
			DropShadowLayer{
				XOffset: int(math.Round(float64(layer.XOffset) * scale)),
				YOffset: int(math.Round(float64(layer.YOffset) * scale)),
				Blur:    layer.Blur * scale,
				Opacity: layer.Opacity,
			},
		)
	}

	return scaledLayers
}

func dropShadowRequiredInsets(layers []DropShadowLayer) (int, int, int, int) {
	requiredLeft := 0.0
	requiredRight := 0.0
	requiredTop := 0.0
	requiredBottom := 0.0

	for _, layer := range layers {
		radius := dropShadowRadiusForInset(layer.Blur, layer.Opacity)
		if radius == 0 {
			continue
		}

		requiredLeft = max(requiredLeft, max(0.0, radius-float64(layer.XOffset)))
		requiredRight = max(requiredRight, max(0.0, radius+float64(layer.XOffset)))
		requiredTop = max(requiredTop, max(0.0, radius-float64(layer.YOffset)))
		requiredBottom = max(requiredBottom, max(0.0, radius+float64(layer.YOffset)))
	}

	return int(math.Ceil(requiredLeft)),
		int(math.Ceil(requiredRight)),
		int(math.Ceil(requiredTop)),
		int(math.Ceil(requiredBottom))
}

func dropShadowRadiusForInset(blur float64, opacity float64) float64 {
	if blur <= 0 || opacity <= dropShadowVisibleAlphaCutoff {
		return 0
	}

	return blur * math.Sqrt(-2*math.Log(dropShadowVisibleAlphaCutoff/opacity))
}

func resolveInsetOffset(availableInset int, requiredStart int, requiredEnd int) int {
	if availableInset <= 0 {
		return 0
	}

	requiredTotal := requiredStart + requiredEnd
	if requiredTotal <= 0 {
		return availableInset / 2
	}

	if requiredTotal <= availableInset {
		return requiredStart + (availableInset-requiredTotal)/2
	}

	offset := int(math.Round(float64(availableInset) * (float64(requiredStart) / float64(requiredTotal))))

	return max(0, min(availableInset, offset))
}
