package main

import (
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/codegangsta/cli"
)

var borderColor string
var outputDir string
var edgeThreshold float64
var minDPI float64
var targetWidth float64
var targetHeight float64
var borderWidth float64
var bleedWidth float64

func main() {
	if len(os.Args) < 2 {
		fmt.Println("nothing to do (no files specified)")
		return
	}

	app := cli.NewApp()
	app.Name = "cardresize"
	app.Usage = "cardresize <image1> <image2> <etc>"
	app.Action = run

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "border",
			Value:       "detect",
			Usage:       "rgb hex color for the border, or \"detect\" for autodetect",
			Destination: &borderColor,
		},
		cli.StringFlag{
			Name:        "output",
			Value:       "output",
			Usage:       "output directory",
			Destination: &outputDir,
		},
		cli.Float64Flag{
			Name:        "edge-threshold",
			Value:       0.1,
			Usage:       "threshold for edge detection",
			Destination: &edgeThreshold,
		},
		cli.Float64Flag{
			Name:        "target-width",
			Value:       6.3 / 2.54,
			Usage:       "minimum width (inches) to size the image up to, if it's too small",
			Destination: &targetWidth,
		},
		cli.Float64Flag{
			Name:        "target-height",
			Value:       8.8 / 2.54,
			Usage:       "minimum height (inches) to size the image up to, if it's too small",
			Destination: &targetHeight,
		},
		cli.Float64Flag{
			Name:        "dpi",
			Value:       300,
			Usage:       "minimum dpi to scale up to if needed",
			Destination: &minDPI,
		},
		cli.Float64Flag{
			Name:        "border-width",
			Value:       3.0 / 32,
			Usage:       "width (inches) of the border to apply",
			Destination: &borderWidth,
		},
		cli.Float64Flag{
			Name:        "bleed-width",
			Value:       0.125,
			Usage:       "width (inches) of bleed to account for",
			Destination: &bleedWidth,
		},
	}

	app.Run(os.Args)
}

func run(c *cli.Context) {
	os.MkdirAll(outputDir, os.FileMode(0755))
	for _, inFilename := range c.Args() {
		fmt.Println(inFilename + " ->")
		outFilename, err := convertImage(inFilename)
		if err == nil {
			fmt.Println(outFilename)
		} else {
			fmt.Println("ERROR")
			fmt.Println(err)
		}
	}
}

func convertImage(inFilename string) (string, error) {
	rawImage, err := loadImage(inFilename)
	if err != nil {
		return "", err
	}

	clipRect, needsRotation, fillColor, err := analyzeImage(rawImage)
	if err != nil {
		return "", err
	}

	resized, err := resizeImage(rawImage, clipRect, needsRotation, fillColor)
	if err != nil {
		return "", err
	}

	outFilename := fmt.Sprintf("%s%c%s.png", outputDir, filepath.Separator, path.Base(inFilename[0:len(inFilename)-len(path.Ext(inFilename))]))
	err = saveImage(outFilename, resized)

	return outFilename, err
}

func analyzeImage(img image.Image) (image.Rectangle, bool, color.Color, error) {
	clipRect, fillColor, err := detectBorder(img)
	if err != nil {
		return clipRect, false, fillColor, err
	}
	if borderColor != "detect" {
		c, err := parseColor(borderColor)
		if err != nil {
			return clipRect, false, fillColor, err
		}
		fillColor = color.Color(c)
	}
	needsRotation := clipRect.Bounds().Max.X-clipRect.Bounds().Min.X > clipRect.Bounds().Max.Y-clipRect.Bounds().Min.Y
	fmt.Printf("\tbounds = %v\n", img.Bounds())
	fmt.Printf("\tclip = %v\n", clipRect)
	return clipRect, needsRotation, fillColor, nil
}

var mc = math.Sqrt(math.Pow(255, 2) * 3)

func colorDistance(c1 color.Color, c2 color.Color) float64 {
	r1, r2 := rgba(c1), rgba(c2)
	return math.Sqrt(math.Pow(float64(r2.R)-float64(r1.R), 2)+math.Pow(float64(r2.G)-float64(r1.G), 2)+math.Pow(float64(r2.B)-float64(r1.B), 2)) / mc
}

func averageColor(colors []color.Color) color.Color {
	sr, sg, sb := uint64(0), uint64(0), uint64(0)
	for _, c := range colors {
		r, g, b, _ := c.RGBA()
		sr += uint64(r)
		sg += uint64(g)
		sb += uint64(b)
	}
	return color.RGBA{
		R: uint8((sr / uint64(len(colors))) >> 8),
		G: uint8((sg / uint64(len(colors))) >> 8),
		B: uint8((sb / uint64(len(colors))) >> 8),
		A: 0xFF,
	}
}

func rgba(c color.Color) color.RGBA {
	r, g, b, _ := c.RGBA()
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: 0xFF,
	}
}

func average(ints ...int) int {
	s := 0
	for _, n := range ints {
		s += n
	}
	return s / len(ints)
}

func detectEdge(img image.Image, xStart int, xEnd int, yStart int, yEnd int, xStep int, yStep int) (int, int, color.Color, error) {
	colors := []color.Color{rgba(img.At(xStart, yStart))}
	for x, y := xStart, yStart; x != xEnd-xStep && y != yEnd-yStep; x, y = x+xStep, y+yStep {
		next := rgba(img.At(x+xStep, y+yStep))
		avg := averageColor(colors)
		dist := colorDistance(next, avg)
		if dist >= edgeThreshold {
			return x, y, avg, nil
		}
		colors = append(colors, next)
		if len(colors) > 3 {
			colors = colors[len(colors)-3 : len(colors)-1]
		}
	}
	return 0, 0, nil, fmt.Errorf("I couldn't figure out where one of the borders was - did you give me a monochrome image?")
}

func detectEdgeByQuarters(img image.Image, xStart int, xEnd int, yStart int, yEnd int, xStep int, yStep int, xInc int, yInc int) (int, int, color.Color, error) {
	x1, y1, c1, err := detectEdge(img, xInc*1+xStart, xEnd, yInc*1+yStart, yEnd, xStep, yStep)
	if err != nil {
		return 0, 0, nil, err
	}
	x2, y2, c2, err := detectEdge(img, xInc*2+xStart, xEnd, yInc*2+yStart, yEnd, xStep, yStep)
	if err != nil {
		return 0, 0, nil, err
	}
	x3, y3, c3, err := detectEdge(img, xInc*3+xStart, xEnd, yInc*3+yStart, yEnd, xStep, yStep)
	if err != nil {
		return 0, 0, nil, err
	}
	return average(x1, x2, x3), average(y1, y2, y3), averageColor([]color.Color{c1, c2, c3}), nil
}

func detectBorder(img image.Image) (image.Rectangle, color.Color, error) {
	xq := img.Bounds().Max.X / 4
	yq := img.Bounds().Max.Y / 4
	left, _, lc, err := detectEdgeByQuarters(img, 0, img.Bounds().Max.X-1, 0, img.Bounds().Max.Y-1, 1, 0, 0, yq)
	if err != nil {
		return image.Rectangle{}, nil, err
	}
	right, _, rc, err := detectEdgeByQuarters(img, img.Bounds().Max.X-1, 0, 0, img.Bounds().Max.Y-1, -1, 0, 0, yq)
	if err != nil {
		return image.Rectangle{}, nil, err
	}
	_, top, tc, err := detectEdgeByQuarters(img, 0, img.Bounds().Max.X-1, 0, img.Bounds().Max.Y-1, 0, 1, xq, 0)
	if err != nil {
		return image.Rectangle{}, nil, err
	}
	_, bottom, bc, err := detectEdgeByQuarters(img, 0, img.Bounds().Max.X-1, img.Bounds().Max.Y-1, 0, 0, -1, xq, 0)
	if err != nil {
		return image.Rectangle{}, nil, err
	}
	return image.Rect(left, top, right, bottom), averageColor([]color.Color{lc, rc, tc, bc}), nil
}

func parseColor(hexCode string) (color.RGBA, error) {
	rgba := color.RGBA{}
	if len(hexCode)%3 != 0 {
		return rgba, fmt.Errorf("idk how to even deal with this hex value you gave me, I was expecting rgb or rrggbb, geez")
	}
	chunkSize := len(hexCode) / 3
	components := [3]uint8{}
	for i := 0; i < 3; i++ {
		chunk := hexCode[i*chunkSize : (i+1)*chunkSize]
		c, err := strconv.ParseUint(padComponent(chunk), 16, 8)
		if err != nil {
			return rgba, err
		}
		components[i] = uint8(c)
	}
	rgba.R = components[0]
	rgba.G = components[1]
	rgba.B = components[2]
	rgba.A = 0xFF
	return rgba, nil
}

func padComponent(c string) string {
	if len(c) == 1 {
		return c + c
	}
	return c[0:2]
}

func resizeImage(source image.Image, clipRect image.Rectangle, needsRotation bool, fillColor color.Color) (*image.RGBA, error) {
	desiredInnerWidth := targetWidth - 2*borderWidth
	desiredInnerHeight := targetHeight - 2*borderWidth
	desiredRatio := desiredInnerWidth / desiredInnerHeight
	width := clipRect.Bounds().Max.X - clipRect.Bounds().Min.X
	height := clipRect.Bounds().Max.Y - clipRect.Bounds().Min.Y
	if needsRotation {
		width, height = height, width
	}
	actualRatio := float64(width) / float64(height)
	calcWidth, calcHeight := width, height
	if actualRatio < desiredRatio { // height is bigger - width needs to increase
		calcWidth = int(math.Floor(float64(height) * desiredRatio))
	} else if actualRatio > desiredRatio { // width is bigger - height needs to increase
		calcHeight = int(math.Floor(float64(width) / desiredRatio))
	}

	actualDPI := float64(calcWidth) / desiredInnerWidth
	fmt.Printf("\tsource DPI = %f\n", actualDPI)
	offset := int(math.Floor((borderWidth + bleedWidth) * actualDPI))
	fullWidth := calcWidth + 2*offset
	fullHeight := calcHeight + 2*offset

	img := image.NewRGBA(image.Rect(0, 0, fullWidth, fullHeight))
	fillImage(img, fillColor)

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			xp, yp := x, y
			if needsRotation {
				xp, yp = yp, width-xp
			}
			img.Set(x+offset, y+offset, source.At(
				clipRect.Bounds().Min.X+xp,
				clipRect.Bounds().Min.Y+yp,
			))
		}
	}

	if actualDPI < minDPI {
	}

	return img, nil
}

func fillImage(img *image.RGBA, fillColor color.Color) {
	for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			img.Set(x, y, fillColor)
		}
	}
}

func loadImage(inFilename string) (image.Image, error) {
	ext := strings.ToLower(path.Ext(inFilename))
	f := png.Decode
	if ext == ".jpg" || ext == ".jpeg" {
		f = jpeg.Decode
	} else if ext == ".gif" {
		f = gif.Decode
	} else if ext != ".png" {
		return nil, fmt.Errorf("no idea what to do with extension \"%s\"", ext)
	}
	r, err := os.Open(inFilename)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return f(r)
}

func saveImage(filename string, img *image.RGBA) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
