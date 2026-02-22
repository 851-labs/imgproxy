package vips

/*
#include "options.h"
*/
import "C"
import (
	"github.com/imgproxy/imgproxy/v3/options"
	"github.com/imgproxy/imgproxy/v3/options/keys"
)

func newLoadOptions(shrink float64, page, pages int) C.ImgproxyLoadOptions {
	return C.ImgproxyLoadOptions{
		Shrink:    C.double(shrink),
		Thumbnail: 0, // Don't load thumbnail by default. Set it explicitly when needed.

		Page:  C.int(page),
		Pages: C.int(pages),

		PngUnlimited: gbool(config.PngUnlimited),
		SvgUnlimited: gbool(config.SvgUnlimited),
	}
}

func newSaveOptions(o *options.Options) C.ImgproxySaveOptions {
	pngInterlaced := config.PngInterlaced
	pngQuantize := config.PngQuantize
	pngQuantizationColors := config.PngQuantizationColors

	if o != nil {
		mainOptions := o.Main()
		pngInterlaced = mainOptions.GetBool(keys.PngOptionsInterlaced, pngInterlaced)
		pngQuantize = mainOptions.GetBool(keys.PngOptionsQuantize, pngQuantize)
		pngQuantizationColors = mainOptions.GetInt(keys.PngOptionsQuantizationColors, pngQuantizationColors)
	}

	return C.ImgproxySaveOptions{
		JpegProgressive: gbool(config.JpegProgressive),

		PngInterlaced:         gbool(pngInterlaced),
		PngQuantize:           gbool(pngQuantize),
		PngQuantizationColors: C.int(pngQuantizationColors),

		WebpPreset: C.VipsForeignWebpPreset(config.WebpPreset),
		WebpEffort: C.int(config.WebpEffort),

		AvifSpeed: C.int(config.AvifSpeed),

		JxlEffort: C.int(config.JxlEffort),
	}
}
