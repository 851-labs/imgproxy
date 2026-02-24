package auximageprovider

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/imgproxy/imgproxy/v3/fetcher"
	"github.com/imgproxy/imgproxy/v3/httpheaders"
	"github.com/imgproxy/imgproxy/v3/imagedata"
	"github.com/imgproxy/imgproxy/v3/options"
	"github.com/imgproxy/imgproxy/v3/options/keys"
	"github.com/imgproxy/imgproxy/v3/testutil"
	"github.com/stretchr/testify/suite"
)

type WatermarkProviderTestSuite struct {
	testutil.LazySuite

	staticData  []byte
	dynamicData []byte

	testServer testutil.LazyTestServer
	idf        *imagedata.Factory
}

func (s *WatermarkProviderTestSuite) SetupSuite() {
	tdp := testutil.NewTestDataProvider(s.T)
	s.staticData = tdp.Read("test1.jpg")
	s.dynamicData = tdp.Read("test2.jpg")

	fc := fetcher.NewDefaultConfig()
	fc.Transport.HTTP.AllowLoopbackSourceAddresses = true

	f, err := fetcher.New(&fc)
	s.Require().NoError(err)

	s.idf = imagedata.NewFactory(f, nil)

	s.testServer, _ = testutil.NewLazySuiteTestServer(
		s,
		func(srv *testutil.TestServer) error {
			srv.SetHeaders(
				httpheaders.ContentType, "image/jpeg",
				httpheaders.ContentLength, strconv.Itoa(len(s.dynamicData)),
			).SetBody(s.dynamicData)

			return nil
		},
	)
}

func (s *WatermarkProviderTestSuite) SetupSubTest() {
	s.ResetLazyObjects()
}

func (s *WatermarkProviderTestSuite) readImageData(data imagedata.ImageData) []byte {
	s.Require().NotNil(data)
	defer data.Close()

	b, err := io.ReadAll(data.Reader())
	s.Require().NoError(err)

	return b
}

func (s *WatermarkProviderTestSuite) TestGetStaticFallback() {
	provider, err := NewWatermarkProvider(
		s.T().Context(),
		&StaticConfig{Path: "../testdata/test1.jpg"},
		s.idf,
	)
	s.Require().NoError(err)

	data, _, err := provider.Get(s.T().Context(), options.New())
	s.Require().NoError(err)

	s.Equal(s.staticData, s.readImageData(data))
}

func (s *WatermarkProviderTestSuite) TestGetDynamicWatermarkURL() {
	provider, err := NewWatermarkProvider(
		s.T().Context(),
		&StaticConfig{Path: "../testdata/test1.jpg"},
		s.idf,
	)
	s.Require().NoError(err)

	o := options.New()
	o.Set(keys.WatermarkURL, s.testServer().URL())

	data, _, err := provider.Get(s.T().Context(), o)
	s.Require().NoError(err)

	s.Equal(s.dynamicData, s.readImageData(data))
}

func (s *WatermarkProviderTestSuite) TestGetEmptyWatermarkURLFallsBackToStatic() {
	provider, err := NewWatermarkProvider(
		s.T().Context(),
		&StaticConfig{Path: "../testdata/test1.jpg"},
		s.idf,
	)
	s.Require().NoError(err)

	o := options.New()
	o.Set(keys.WatermarkURL, "")

	data, _, err := provider.Get(s.T().Context(), o)
	s.Require().NoError(err)

	s.Equal(s.staticData, s.readImageData(data))
}

func (s *WatermarkProviderTestSuite) TestGetDynamicWatermarkURLWithoutStaticFallback() {
	provider, err := NewWatermarkProvider(s.T().Context(), &StaticConfig{}, s.idf)
	s.Require().NoError(err)

	o := options.New()
	o.Set(keys.WatermarkURL, s.testServer().URL())

	data, _, err := provider.Get(s.T().Context(), o)
	s.Require().NoError(err)

	s.Equal(s.dynamicData, s.readImageData(data))
}

func (s *WatermarkProviderTestSuite) TestGetDynamicWatermarkURLInvalid() {
	provider, err := NewWatermarkProvider(
		s.T().Context(),
		&StaticConfig{Path: "../testdata/test1.jpg"},
		s.idf,
	)
	s.Require().NoError(err)

	o := options.New()
	o.Set(keys.WatermarkURL, "http://invalid-url-that-does-not-exist.invalid")

	_, _, err = provider.Get(s.T().Context(), o)
	s.Require().Error(err)
	s.Require().Contains(strings.ToLower(err.Error()), "can't download watermark")
}

func TestWatermarkProvider(t *testing.T) {
	suite.Run(t, new(WatermarkProviderTestSuite))
}
