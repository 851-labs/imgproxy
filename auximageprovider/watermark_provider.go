package auximageprovider

import (
	"context"
	"net/http"

	"github.com/imgproxy/imgproxy/v3/imagedata"
	"github.com/imgproxy/imgproxy/v3/options"
	"github.com/imgproxy/imgproxy/v3/options/keys"
)

// watermarkProvider returns a per-request watermark URL when provided,
// otherwise falls back to static watermark config.
type watermarkProvider struct {
	fallback Provider
	idf      *imagedata.Factory
}

// NewWatermarkProvider creates a watermark provider that supports per-request
// URL override via the watermark_url option while preserving static fallback behavior.
func NewWatermarkProvider(
	ctx context.Context,
	c *StaticConfig,
	idf *imagedata.Factory,
) (Provider, error) {
	fallback, err := NewStaticProvider(ctx, c, "watermark", idf)
	if err != nil {
		return nil, err
	}

	if idf == nil {
		return fallback, nil
	}

	return &watermarkProvider{
		fallback: fallback,
		idf:      idf,
	}, nil
}

func (p *watermarkProvider) Get(
	ctx context.Context,
	opts *options.Options,
) (imagedata.ImageData, http.Header, error) {
	if opts != nil && opts.Has(keys.WatermarkURL) {
		watermarkURL := opts.GetString(keys.WatermarkURL, "")
		if len(watermarkURL) > 0 {
			return p.idf.DownloadSync(
				ctx,
				watermarkURL,
				"watermark",
				imagedata.DownloadOptions{},
			)
		}
	}

	if p.fallback == nil {
		return nil, nil, nil
	}

	return p.fallback.Get(ctx, opts)
}
