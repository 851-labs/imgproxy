package processing

func (p *Processor) applyFilters(c *Context) error {
	blur := c.PO.Blur()
	brightness := c.PO.Brightness()
	saturation := c.PO.Saturation()
	sharpen := c.PO.Sharpen()
	pixelate := c.PO.Pixelate()

	if blur == 0 && brightness == 0 && saturation == 1.0 && sharpen == 0 && pixelate <= 1 {
		return nil
	}

	if err := c.Img.CopyMemory(); err != nil {
		return err
	}

	if err := c.Img.RgbColourspace(); err != nil {
		return err
	}

	if err := c.Img.ApplyFilters(blur, sharpen, pixelate, brightness, saturation); err != nil {
		return err
	}

	return c.Img.CopyMemory()
}
