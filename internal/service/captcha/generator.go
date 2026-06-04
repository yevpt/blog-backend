package captcha

import (
	"errors"

	"github.com/wenlng/go-captcha-assets/resources/imagesv2"
	"github.com/wenlng/go-captcha-assets/resources/tiles"
	"github.com/wenlng/go-captcha/v2/base/option"
	"github.com/wenlng/go-captcha/v2/slide"
)

var errEmptyCaptchaData = errors.New("GoCaptcha 生成数据为空")

type slideGenerator interface {
	Generate() (*slideChallenge, error)
}

type slideChallenge struct {
	MasterImage string
	TileImage   string
	X           int
	Y           int
	TileX       int
	TileY       int
	Width       int
	Height      int
	ImageWidth  int
	ImageHeight int
}

type goCaptchaSlideGenerator struct {
	capt slide.Captcha
}

func newGoCaptchaSlideGenerator() (*goCaptchaSlideGenerator, error) {
	// 使用官方资源包，避免项目内维护二进制图片素材。
	backgrounds, err := imagesv2.GetImages()
	if err != nil {
		return nil, err
	}

	graphs, err := tiles.GetTiles()
	if err != nil {
		return nil, err
	}

	// 将资源包图形适配成 go-captcha/v2 的 slide.GraphImage。
	slideGraphs := make([]*slide.GraphImage, 0, len(graphs))
	for _, graph := range graphs {
		slideGraphs = append(slideGraphs, &slide.GraphImage{
			OverlayImage: graph.OverlayImage,
			ShadowImage:  graph.ShadowImage,
			MaskImage:    graph.MaskImage,
		})
	}

	builder := slide.NewBuilder(
		slide.WithImageSize(option.Size{Width: 300, Height: 220}),
		slide.WithRangeGraphSize(option.RangeVal{Min: 56, Max: 64}),
	)
	builder.SetResources(
		slide.WithBackgrounds(backgrounds),
		slide.WithGraphImages(slideGraphs),
	)

	return &goCaptchaSlideGenerator{capt: builder.Make()}, nil
}

func (g *goCaptchaSlideGenerator) Generate() (*slideChallenge, error) {
	captData, err := g.capt.Generate()
	if err != nil {
		return nil, err
	}

	block := captData.GetData()
	if block == nil {
		return nil, errEmptyCaptchaData
	}

	masterImage, err := captData.GetMasterImage().ToBase64()
	if err != nil {
		return nil, err
	}
	tileImage, err := captData.GetTileImage().ToBase64()
	if err != nil {
		return nil, err
	}

	size := g.capt.GetOptions().GetImageSize()
	return &slideChallenge{
		MasterImage: masterImage,
		TileImage:   tileImage,
		X:           block.X,
		Y:           block.Y,
		TileX:       block.DX,
		TileY:       block.DY,
		Width:       block.Width,
		Height:      block.Height,
		ImageWidth:  size.Width,
		ImageHeight: size.Height,
	}, nil
}
