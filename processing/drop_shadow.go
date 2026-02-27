package processing

func (p *Processor) dropShadow(c *Context) error {
	layers := c.PO.DropShadowLayers()
	if len(layers) == 0 || !c.Img.HasAlpha() {
		return nil
	}

	if err := c.Img.CopyMemory(); err != nil {
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
