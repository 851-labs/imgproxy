package processing

import "math"

const dropShadowBlurInsetFactor = 4.0

func (p *Processor) dropShadow(c *Context) error {
	layers := c.PO.DropShadowLayers()
	if len(layers) == 0 || !c.Img.HasAlpha() {
		return nil
	}

	if err := c.Img.CopyMemory(); err != nil {
		return err
	}

	if err := insetImageForDropShadow(c, layers); err != nil {
		return err
	}

	xOffsets := make([]int, 0, len(layers))
	yOffsets := make([]int, 0, len(layers))
	blurs := make([]float64, 0, len(layers))
	opacities := make([]float64, 0, len(layers))

	for _, layer := range layers {
		xOffsets = append(xOffsets, layer.XOffset)
		yOffsets = append(yOffsets, layer.YOffset)
		blurs = append(blurs, layer.Blur)
		opacities = append(opacities, layer.Opacity)
	}

	return c.Img.ApplyDropShadow(xOffsets, yOffsets, blurs, opacities)
}

func insetImageForDropShadow(c *Context, layers []DropShadowLayer) error {
	requiredLeft, requiredRight, requiredTop, requiredBottom := dropShadowRequiredInsets(layers)

	imageWidth := c.Img.Width()
	imageHeight := c.Img.Height()

	alphaLeft, alphaTop, alphaWidth, alphaHeight, err := c.Img.AlphaBounds()
	if err != nil {
		return err
	}

	if alphaWidth == 0 || alphaHeight == 0 {
		return nil
	}

	existingLeft := alphaLeft
	existingTop := alphaTop
	existingRight := imageWidth - alphaLeft - alphaWidth
	existingBottom := imageHeight - alphaTop - alphaHeight

	insetLeft := max(0, requiredLeft-existingLeft)
	insetRight := max(0, requiredRight-existingRight)
	insetTop := max(0, requiredTop-existingTop)
	insetBottom := max(0, requiredBottom-existingBottom)

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

	offsetX := insetLeft + max(0, (availableHorizontalInset-insetLeft-insetRight)/2)
	offsetY := insetTop + max(0, (availableVerticalInset-insetTop-insetBottom)/2)

	return c.Img.Embed(imageWidth, imageHeight, offsetX, offsetY)
}

func dropShadowRequiredInsets(layers []DropShadowLayer) (int, int, int, int) {
	requiredLeft := 0.0
	requiredRight := 0.0
	requiredTop := 0.0
	requiredBottom := 0.0

	for _, layer := range layers {
		radius := layer.Blur * dropShadowBlurInsetFactor

		requiredLeft += max(0.0, radius-float64(layer.XOffset))
		requiredRight += max(0.0, radius+float64(layer.XOffset))
		requiredTop += max(0.0, radius-float64(layer.YOffset))
		requiredBottom += max(0.0, radius+float64(layer.YOffset))
	}

	return int(math.Ceil(requiredLeft)),
		int(math.Ceil(requiredRight)),
		int(math.Ceil(requiredTop)),
		int(math.Ceil(requiredBottom))
}
