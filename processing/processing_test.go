package processing

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"testing"

	"github.com/imgproxy/imgproxy/v3/errctx"
	"github.com/imgproxy/imgproxy/v3/imagedata"
	"github.com/imgproxy/imgproxy/v3/imagetype"
	"github.com/imgproxy/imgproxy/v3/options"
	"github.com/imgproxy/imgproxy/v3/options/keys"
	"github.com/imgproxy/imgproxy/v3/testutil"
	"github.com/stretchr/testify/suite"
)

type ProcessingTestSuite struct {
	testSuite

	img imagedata.ImageData
}

type sizeLimitTestCase struct {
	limit         int
	width         int
	height        int
	resizingType  ResizeType
	enlarge       bool
	extend        bool
	extendAR      bool
	paddingTop    int
	paddingRight  int
	paddingBottom int
	paddingLeft   int
	rotate        int
}

func (r sizeLimitTestCase) Set(o *options.Options) {
	o.Set(keys.MaxResultDimension, r.limit)
	o.Set(keys.Width, r.width)
	o.Set(keys.Height, r.height)
	o.Set(keys.ResizingType, r.resizingType)
	o.Set(keys.Enlarge, r.enlarge)
	o.Set(keys.ExtendEnabled, r.extend)
	o.Set(keys.ExtendAspectRatioEnabled, r.extendAR)
	o.Set(keys.Rotate, r.rotate)
	o.Set(keys.PaddingTop, r.paddingTop)
	o.Set(keys.PaddingRight, r.paddingRight)
	o.Set(keys.PaddingBottom, r.paddingBottom)
	o.Set(keys.PaddingLeft, r.paddingLeft)
}

func (r sizeLimitTestCase) String() string {
	b := bytes.NewBuffer(nil)

	fmt.Fprintf(b, "%s:%dx%d:limit:%d", r.resizingType, r.width, r.height, r.limit)

	if r.enlarge {
		fmt.Fprintf(b, "_en:%t", r.enlarge)
	}

	if r.extend {
		fmt.Fprintf(b, "_ex:%t", r.extend)
	}

	if r.extendAR {
		fmt.Fprintf(b, "_exAR:%t", r.extendAR)
	}

	if r.rotate != 0 {
		fmt.Fprintf(b, "_rotate:%d", r.rotate)
	}

	if r.paddingTop > 0 || r.paddingRight > 0 || r.paddingBottom > 0 || r.paddingLeft > 0 {
		fmt.Fprintf(
			b, "_padding:%dx%dx%dx%d",
			r.paddingTop, r.paddingRight, r.paddingBottom, r.paddingLeft,
		)
	}

	return b.String()
}

func (s *ProcessingTestSuite) SetupSuite() {
	s.testSuite.SetupSuite()

	var err error

	s.img, err = s.ImageDataFactory().NewFromPath(s.TestData.Path("geometry.png"))
	s.Require().NoError(err)
}

func (s *ProcessingTestSuite) TestResizeToFit() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFit)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 25}},
		{opts: testSize{50, 20}, outSize: testSize{40, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 10}},
		{opts: testSize{300, 300}, outSize: testSize{200, 100}},
		{opts: testSize{300, 50}, outSize: testSize{100, 50}},
		{opts: testSize{100, 300}, outSize: testSize{100, 50}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{200, 100}},
		{opts: testSize{300, 0}, outSize: testSize{200, 100}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFitEnlarge() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFit)
	o.Set(keys.Enlarge, true)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 25}},
		{opts: testSize{50, 20}, outSize: testSize{40, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 10}},
		{opts: testSize{300, 300}, outSize: testSize{300, 150}},
		{opts: testSize{300, 125}, outSize: testSize{250, 125}},
		{opts: testSize{250, 300}, outSize: testSize{250, 125}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{400, 200}},
		{opts: testSize{300, 0}, outSize: testSize{300, 150}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFitExtend() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFit)
	o.Set(keys.ExtendEnabled, true)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{300, 300}},
		{opts: testSize{300, 125}, outSize: testSize{300, 125}},
		{opts: testSize{250, 300}, outSize: testSize{250, 300}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{200, 200}},
		{opts: testSize{300, 0}, outSize: testSize{300, 100}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFitExtendAR() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFit)
	o.Set(keys.ExtendAspectRatioEnabled, true)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{200, 200}},
		{opts: testSize{300, 125}, outSize: testSize{240, 100}},
		{opts: testSize{250, 500}, outSize: testSize{200, 400}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{200, 100}},
		{opts: testSize{300, 0}, outSize: testSize{200, 100}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFill() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFill)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{200, 100}},
		{opts: testSize{300, 50}, outSize: testSize{200, 50}},
		{opts: testSize{100, 300}, outSize: testSize{100, 100}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{200, 100}},
		{opts: testSize{300, 0}, outSize: testSize{200, 100}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFillEnlarge() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFill)
	o.Set(keys.Enlarge, true)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{300, 300}},
		{opts: testSize{300, 125}, outSize: testSize{300, 125}},
		{opts: testSize{250, 300}, outSize: testSize{250, 300}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{400, 200}},
		{opts: testSize{300, 0}, outSize: testSize{300, 150}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFillExtend() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFill)
	o.Set(keys.ExtendEnabled, true)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{300, 300}},
		{opts: testSize{300, 125}, outSize: testSize{300, 125}},
		{opts: testSize{250, 300}, outSize: testSize{250, 300}},
		{opts: testSize{300, 50}, outSize: testSize{300, 50}},
		{opts: testSize{100, 300}, outSize: testSize{100, 300}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{200, 200}},
		{opts: testSize{300, 0}, outSize: testSize{300, 100}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFillExtendAR() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFill)
	o.Set(keys.ExtendAspectRatioEnabled, true)
	o.Set(keys.ExtendAspectRatioGravityType, GravityCenter)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{200, 200}},
		{opts: testSize{300, 125}, outSize: testSize{240, 100}},
		{opts: testSize{250, 500}, outSize: testSize{200, 400}},
		{opts: testSize{300, 50}, outSize: testSize{300, 50}},
		{opts: testSize{100, 300}, outSize: testSize{100, 300}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{200, 100}},
		{opts: testSize{300, 0}, outSize: testSize{200, 100}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFillDown() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFillDown)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{100, 100}},
		{opts: testSize{300, 125}, outSize: testSize{200, 83}},
		{opts: testSize{250, 300}, outSize: testSize{83, 100}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{200, 100}},
		{opts: testSize{300, 0}, outSize: testSize{200, 100}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFillDownEnlarge() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFillDown)
	o.Set(keys.Enlarge, true)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{300, 300}},
		{opts: testSize{300, 125}, outSize: testSize{300, 125}},
		{opts: testSize{250, 300}, outSize: testSize{250, 300}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{400, 200}},
		{opts: testSize{300, 0}, outSize: testSize{300, 150}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFillDownExtend() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFillDown)
	o.Set(keys.ExtendEnabled, true)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{300, 300}},
		{opts: testSize{300, 125}, outSize: testSize{300, 125}},
		{opts: testSize{250, 300}, outSize: testSize{250, 300}},
		{opts: testSize{300, 50}, outSize: testSize{300, 50}},
		{opts: testSize{100, 300}, outSize: testSize{100, 300}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{200, 200}},
		{opts: testSize{300, 0}, outSize: testSize{300, 100}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResizeToFillDownExtendAR() {
	o := options.New()
	o.Set(keys.ResizingType, ResizeFillDown)
	o.Set(keys.ExtendAspectRatioEnabled, true)

	testCases := []testCase[testSize]{
		{opts: testSize{50, 50}, outSize: testSize{50, 50}},
		{opts: testSize{50, 20}, outSize: testSize{50, 20}},
		{opts: testSize{20, 50}, outSize: testSize{20, 50}},
		{opts: testSize{300, 300}, outSize: testSize{100, 100}},
		{opts: testSize{300, 125}, outSize: testSize{200, 83}},
		{opts: testSize{250, 300}, outSize: testSize{83, 100}},
		{opts: testSize{0, 50}, outSize: testSize{100, 50}},
		{opts: testSize{50, 0}, outSize: testSize{50, 25}},
		{opts: testSize{0, 200}, outSize: testSize{200, 100}},
		{opts: testSize{300, 0}, outSize: testSize{200, 100}},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestResultSizeLimit() {
	testCases := []testCase[sizeLimitTestCase]{
		{
			opts: sizeLimitTestCase{
				limit:        1000,
				width:        100,
				height:       100,
				resizingType: ResizeFit,
			},
			outSize: testSize{100, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        100,
				height:       100,
				resizingType: ResizeFit,
			},
			outSize: testSize{50, 25},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        0,
				height:       0,
				resizingType: ResizeFit,
			},
			outSize: testSize{50, 25},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        0,
				height:       100,
				resizingType: ResizeFit,
			},
			outSize: testSize{100, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        150,
				height:       0,
				resizingType: ResizeFit,
			},
			outSize: testSize{50, 25},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       1000,
				resizingType: ResizeFit,
			},
			outSize: testSize{100, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       1000,
				resizingType: ResizeFit,
				enlarge:      true,
			},
			outSize: testSize{100, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       2000,
				resizingType: ResizeFit,
				extend:       true,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       2000,
				resizingType: ResizeFit,
				extendAR:     true,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        100,
				height:       150,
				resizingType: ResizeFit,
				rotate:       90,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        0,
				height:       0,
				resizingType: ResizeFit,
				rotate:       90,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:         200,
				width:         100,
				height:        100,
				resizingType:  ResizeFit,
				paddingTop:    100,
				paddingRight:  200,
				paddingBottom: 300,
				paddingLeft:   400,
			},
			outSize: testSize{200, 129},
		},
		{
			opts: sizeLimitTestCase{
				limit:        1000,
				width:        100,
				height:       100,
				resizingType: ResizeFill,
			},
			outSize: testSize{100, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        100,
				height:       100,
				resizingType: ResizeFill,
			},
			outSize: testSize{50, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        1000,
				height:       50,
				resizingType: ResizeFill,
			},
			outSize: testSize{50, 13},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        100,
				height:       1000,
				resizingType: ResizeFill,
			},
			outSize: testSize{50, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        0,
				height:       0,
				resizingType: ResizeFill,
			},
			outSize: testSize{50, 25},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        0,
				height:       100,
				resizingType: ResizeFill,
			},
			outSize: testSize{100, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        150,
				height:       0,
				resizingType: ResizeFill,
			},
			outSize: testSize{50, 25},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       1000,
				resizingType: ResizeFill,
			},
			outSize: testSize{100, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       1000,
				resizingType: ResizeFill,
				enlarge:      true,
			},
			outSize: testSize{100, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       2000,
				resizingType: ResizeFill,
				extend:       true,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       2000,
				resizingType: ResizeFill,
				extendAR:     true,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        100,
				height:       150,
				resizingType: ResizeFill,
				rotate:       90,
			},
			outSize: testSize{67, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        0,
				height:       0,
				resizingType: ResizeFill,
				rotate:       90,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:         200,
				width:         100,
				height:        100,
				resizingType:  ResizeFill,
				paddingTop:    100,
				paddingRight:  200,
				paddingBottom: 300,
				paddingLeft:   400,
			},
			outSize: testSize{200, 144},
		},
		{
			opts: sizeLimitTestCase{
				limit:        1000,
				width:        100,
				height:       100,
				resizingType: ResizeFillDown,
			},
			outSize: testSize{100, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        100,
				height:       100,
				resizingType: ResizeFillDown,
			},
			outSize: testSize{50, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        1000,
				height:       50,
				resizingType: ResizeFillDown,
			},
			outSize: testSize{50, 3},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        100,
				height:       1000,
				resizingType: ResizeFillDown,
			},
			outSize: testSize{5, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        0,
				height:       0,
				resizingType: ResizeFillDown,
			},
			outSize: testSize{50, 25},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        0,
				height:       100,
				resizingType: ResizeFillDown,
			},
			outSize: testSize{100, 50},
		},
		{
			opts: sizeLimitTestCase{
				limit:        50,
				width:        150,
				height:       0,
				resizingType: ResizeFillDown,
			},
			outSize: testSize{50, 25},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       1000,
				resizingType: ResizeFillDown,
			},
			outSize: testSize{100, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       1000,
				resizingType: ResizeFillDown,
				enlarge:      true,
			},
			outSize: testSize{100, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       2000,
				resizingType: ResizeFillDown,
				extend:       true,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       2000,
				resizingType: ResizeFillDown,
				extendAR:     true,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        1000,
				height:       1500,
				resizingType: ResizeFillDown,
				rotate:       90,
			},
			outSize: testSize{67, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:        100,
				width:        0,
				height:       0,
				resizingType: ResizeFillDown,
				rotate:       90,
			},
			outSize: testSize{50, 100},
		},
		{
			opts: sizeLimitTestCase{
				limit:         200,
				width:         100,
				height:        100,
				resizingType:  ResizeFillDown,
				paddingTop:    100,
				paddingRight:  200,
				paddingBottom: 300,
				paddingLeft:   400,
			},
			outSize: testSize{200, 144},
		},
		{
			opts: sizeLimitTestCase{
				limit:         200,
				width:         1000,
				height:        1000,
				resizingType:  ResizeFillDown,
				paddingTop:    100,
				paddingRight:  200,
				paddingBottom: 300,
				paddingLeft:   400,
			},
			outSize: testSize{200, 144},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.opts.String(), func() {
			o := options.New()
			tc.opts.Set(o)

			s.processImageAndCheck(s.img, o, tc)
		})
	}
}

func (s *ProcessingTestSuite) TestImageResolutionTooLarge() {
	o := options.New()
	o.Set(keys.MaxSrcResolution, 1)

	_, err := s.Processor().ProcessImage(s.T().Context(), s.img, o)

	s.Require().Error(err)
	s.Require().Equal(422, errctx.Wrap(err).StatusCode())
}

func (s *ProcessingTestSuite) TestStickerTraceMatchesTwoPassTraceThenResize() {
	sourceImage := image.NewNRGBA(image.Rect(0, 0, 512, 512))

	for x := 32; x < 480; x++ {
		sourceImage.SetNRGBA(x, 64, color.NRGBA{R: 226, G: 47, B: 47, A: 255})
		sourceImage.SetNRGBA(x, 448, color.NRGBA{R: 47, G: 150, B: 226, A: 255})
	}

	for y := 64; y < 448; y++ {
		sourceImage.SetNRGBA(64, y, color.NRGBA{R: 47, G: 150, B: 226, A: 255})
		sourceImage.SetNRGBA(448, y, color.NRGBA{R: 226, G: 47, B: 47, A: 255})
	}

	for i := range 512 {
		sourceImage.SetNRGBA(i, i, color.NRGBA{R: 40, G: 217, B: 97, A: 255})
		if i+1 < 512 {
			sourceImage.SetNRGBA(i+1, i, color.NRGBA{R: 40, G: 217, B: 97, A: 255})
		}
	}

	for y := 172; y < 340; y++ {
		for x := 172; x < 340; x++ {
			sourceImage.SetNRGBA(x, y, color.NRGBA{})
		}
	}

	var sourceImageBuffer bytes.Buffer
	err := png.Encode(&sourceImageBuffer, sourceImage)
	s.Require().NoError(err)

	sourceImageBytes := sourceImageBuffer.Bytes()

	tracedImageBytes, err := transformStickerTraceImage(s.T().Context(), sourceImageBytes)
	s.Require().NoError(err)

	sourceImageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, sourceImageBytes)
	defer sourceImageData.Close()

	tracedImageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, tracedImageBytes)
	defer tracedImageData.Close()

	singlePassOptions := options.New()
	singlePassOptions.Set(keys.StickerTrace, true)
	singlePassOptions.Set(keys.Width, 96)
	singlePassOptions.Set(keys.Format, imagetype.PNG)

	singlePassResult, err := s.Processor().ProcessImage(s.T().Context(), sourceImageData, singlePassOptions)
	s.Require().NoError(err)
	defer singlePassResult.OutData.Close()

	twoPassOptions := options.New()
	twoPassOptions.Set(keys.Width, 96)
	twoPassOptions.Set(keys.Format, imagetype.PNG)

	twoPassResult, err := s.Processor().ProcessImage(s.T().Context(), tracedImageData, twoPassOptions)
	s.Require().NoError(err)
	defer twoPassResult.OutData.Close()

	if !testutil.ReadersEqual(
		s.T(),
		twoPassResult.OutData.Reader(),
		singlePassResult.OutData.Reader(),
	) {
		s.T().Fatal("expected single-pass sticker_trace output to match two-pass trace-then-resize output")
	}
}

func (s *ProcessingTestSuite) TestDropShadowNoOpWithoutAlpha() {
	noAlphaImage := image.NewGray(image.Rect(0, 0, 24, 24))

	for y := range 24 {
		for x := range 24 {
			noAlphaImage.SetGray(x, y, color.Gray{Y: uint8(x + y)})
		}
	}

	var encoded bytes.Buffer
	err := png.Encode(&encoded, noAlphaImage)
	s.Require().NoError(err)

	sourceBytes := encoded.Bytes()

	withoutShadowImageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, sourceBytes)
	defer withoutShadowImageData.Close()

	withShadowImageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, sourceBytes)
	defer withShadowImageData.Close()

	withoutShadowOptions := options.New()
	withoutShadowOptions.Set(keys.Format, imagetype.PNG)

	withShadowOptions := options.New()
	withShadowOptions.Set(keys.Format, imagetype.PNG)
	withShadowOptions.Set(keys.DropShadow, []DropShadowLayer{{XOffset: 0, YOffset: 1, Blur: 1, Opacity: 0.5}})

	withoutShadowResult, err := s.Processor().ProcessImage(s.T().Context(), withoutShadowImageData, withoutShadowOptions)
	s.Require().NoError(err)
	defer withoutShadowResult.OutData.Close()

	withShadowResult, err := s.Processor().ProcessImage(s.T().Context(), withShadowImageData, withShadowOptions)
	s.Require().NoError(err)
	defer withShadowResult.OutData.Close()

	if !testutil.ReadersEqual(s.T(), withoutShadowResult.OutData.Reader(), withShadowResult.OutData.Reader()) {
		s.T().Fatal("expected drop_shadow to be a no-op for images without alpha")
	}
}

func (s *ProcessingTestSuite) TestDropShadowZeroOpacityNoOp() {
	alphaImage := image.NewNRGBA(image.Rect(0, 0, 24, 24))

	for y := range 24 {
		for x := range 24 {
			alphaImage.SetNRGBA(x, y, color.NRGBA{R: 226, G: 47, B: 47, A: 255})
		}
	}

	var encoded bytes.Buffer
	err := png.Encode(&encoded, alphaImage)
	s.Require().NoError(err)

	sourceBytes := encoded.Bytes()

	withoutShadowImageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, sourceBytes)
	defer withoutShadowImageData.Close()

	withShadowImageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, sourceBytes)
	defer withShadowImageData.Close()

	withoutShadowOptions := options.New()
	withoutShadowOptions.Set(keys.Format, imagetype.PNG)

	withShadowOptions := options.New()
	withShadowOptions.Set(keys.Format, imagetype.PNG)
	withShadowOptions.Set(
		keys.DropShadow,
		[]DropShadowLayer{
			{XOffset: 0, YOffset: 1, Blur: 1, Opacity: 0},
			{XOffset: 0, YOffset: 1, Blur: 2, Opacity: 0},
		},
	)

	withoutShadowResult, err := s.Processor().ProcessImage(s.T().Context(), withoutShadowImageData, withoutShadowOptions)
	s.Require().NoError(err)
	defer withoutShadowResult.OutData.Close()

	withShadowResult, err := s.Processor().ProcessImage(s.T().Context(), withShadowImageData, withShadowOptions)
	s.Require().NoError(err)
	defer withShadowResult.OutData.Close()

	if !testutil.ReadersEqual(s.T(), withoutShadowResult.OutData.Reader(), withShadowResult.OutData.Reader()) {
		s.T().Fatal("expected zero-opacity drop_shadow layers to be a no-op")
	}
}

func (s *ProcessingTestSuite) TestDropShadowAddsShadowBehindAlphaMask() {
	alphaImage := image.NewNRGBA(image.Rect(0, 0, 24, 24))

	for y := 8; y <= 15; y++ {
		for x := 8; x <= 15; x++ {
			alphaImage.SetNRGBA(x, y, color.NRGBA{R: 226, G: 47, B: 47, A: 255})
		}
	}

	var encoded bytes.Buffer
	err := png.Encode(&encoded, alphaImage)
	s.Require().NoError(err)

	sourceBytes := encoded.Bytes()

	withoutShadowImageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, sourceBytes)
	defer withoutShadowImageData.Close()

	withShadowImageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, sourceBytes)
	defer withShadowImageData.Close()

	withoutShadowOptions := options.New()
	withoutShadowOptions.Set(keys.Format, imagetype.PNG)

	withShadowOptions := options.New()
	withShadowOptions.Set(keys.Format, imagetype.PNG)
	withShadowOptions.Set(keys.DropShadow, []DropShadowLayer{{XOffset: 1, YOffset: 1, Blur: 0, Opacity: 1}})

	withoutShadowResult, err := s.Processor().ProcessImage(s.T().Context(), withoutShadowImageData, withoutShadowOptions)
	s.Require().NoError(err)
	defer withoutShadowResult.OutData.Close()

	withShadowResult, err := s.Processor().ProcessImage(s.T().Context(), withShadowImageData, withShadowOptions)
	s.Require().NoError(err)
	defer withShadowResult.OutData.Close()

	withoutShadowOutput := decodeImageToNRGBA(s.T(), withoutShadowResult.OutData.Reader())
	withShadowOutput := decodeImageToNRGBA(s.T(), withShadowResult.OutData.Reader())

	s.Require().Equal(withoutShadowResult.ResultWidth, withShadowResult.ResultWidth)
	s.Require().Equal(withoutShadowResult.ResultHeight, withShadowResult.ResultHeight)

	withoutShadowPixel := withoutShadowOutput.NRGBAAt(16, 16)
	withShadowPixel := withShadowOutput.NRGBAAt(16, 16)
	withoutShadowForegroundPixel := withoutShadowOutput.NRGBAAt(12, 12)
	withShadowForegroundPixel := withShadowOutput.NRGBAAt(12, 12)

	s.Require().Equal(color.NRGBA{}, withoutShadowPixel)
	s.Require().Equal(color.NRGBA{A: 255}, withShadowPixel)
	s.Require().Equal(withoutShadowForegroundPixel, withShadowForegroundPixel)
}

func (s *ProcessingTestSuite) TestDropShadowInsetsToAvoidClippingOnOpaqueEdges() {
	alphaImage := image.NewNRGBA(image.Rect(0, 0, 24, 24))

	for y := range 24 {
		for x := 2; x < 22; x++ {
			alphaImage.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	var encoded bytes.Buffer
	err := png.Encode(&encoded, alphaImage)
	s.Require().NoError(err)

	imageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, encoded.Bytes())
	defer imageData.Close()

	processingOptions := options.New()
	processingOptions.Set(keys.Format, imagetype.PNG)
	processingOptions.Set(
		keys.DropShadow,
		[]DropShadowLayer{
			{XOffset: 0, YOffset: 1, Blur: 1, Opacity: 0.26},
			{XOffset: 0, YOffset: 1, Blur: 2, Opacity: 0.18},
		},
	)

	result, err := s.Processor().ProcessImage(s.T().Context(), imageData, processingOptions)
	s.Require().NoError(err)
	defer result.OutData.Close()

	output := decodeImageToNRGBA(s.T(), result.OutData.Reader())

	maxForegroundY := -1

	for y := range output.Bounds().Dy() {
		for x := range output.Bounds().Dx() {
			pixel := output.NRGBAAt(x, y)

			if pixel.A < 200 {
				continue
			}

			if pixel.R < 200 || pixel.G < 200 || pixel.B < 200 {
				continue
			}

			maxForegroundY = max(maxForegroundY, y)
		}
	}

	s.Require().GreaterOrEqual(maxForegroundY, 0)
	s.Require().Less(maxForegroundY, output.Bounds().Dy()-1)
}

func (s *ProcessingTestSuite) TestDropShadowCapsInsetShrinkForTinyImages() {
	alphaImage := image.NewNRGBA(image.Rect(0, 0, 96, 96))

	for y := range 96 {
		for x := 8; x < 88; x++ {
			alphaImage.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	var encoded bytes.Buffer
	err := png.Encode(&encoded, alphaImage)
	s.Require().NoError(err)

	imageData := imagedata.NewFromBytesWithFormat(imagetype.PNG, encoded.Bytes())
	defer imageData.Close()

	processingOptions := options.New()
	processingOptions.Set(keys.Format, imagetype.PNG)
	processingOptions.Set(
		keys.DropShadow,
		[]DropShadowLayer{
			{XOffset: 0, YOffset: 3, Blur: 3, Opacity: 0.26},
			{XOffset: 0, YOffset: 3, Blur: 6, Opacity: 0.18},
		},
	)

	result, err := s.Processor().ProcessImage(s.T().Context(), imageData, processingOptions)
	s.Require().NoError(err)
	defer result.OutData.Close()

	output := decodeImageToNRGBA(s.T(), result.OutData.Reader())

	minForegroundY := output.Bounds().Dy()
	maxForegroundY := -1

	for y := range output.Bounds().Dy() {
		for x := range output.Bounds().Dx() {
			pixel := output.NRGBAAt(x, y)

			if pixel.A < 240 {
				continue
			}

			if pixel.R < 240 || pixel.G < 240 || pixel.B < 240 {
				continue
			}

			minForegroundY = min(minForegroundY, y)
			maxForegroundY = max(maxForegroundY, y)
		}
	}

	s.Require().GreaterOrEqual(maxForegroundY, 0)

	foregroundHeight := maxForegroundY - minForegroundY + 1
	minExpectedHeight := int(float64(alphaImage.Bounds().Dy()) * dropShadowMinimumScale)

	s.Require().GreaterOrEqual(foregroundHeight, minExpectedHeight)
}

func decodeImageToNRGBA(t *testing.T, encodedImageBytesReader io.Reader) *image.NRGBA {
	t.Helper()

	decodedImage, _, err := image.Decode(encodedImageBytesReader)
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}

	decodedBounds := decodedImage.Bounds()
	decodedImageNRGBA := image.NewNRGBA(image.Rect(0, 0, decodedBounds.Dx(), decodedBounds.Dy()))
	draw.Draw(decodedImageNRGBA, decodedImageNRGBA.Bounds(), decodedImage, decodedBounds.Min, draw.Src)

	return decodedImageNRGBA
}

func TestProcessing(t *testing.T) {
	suite.Run(t, new(ProcessingTestSuite))
}
