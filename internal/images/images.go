package images

import (
	"image"

	"golang.org/x/image/draw"
)

type subImager interface {
	SubImage(r image.Rectangle) image.Image
}

func SplitByWidth(img image.Image, segmentWidth int) []image.Image {
	bounds := img.Bounds()
	totalWidth := bounds.Dx()

	numSegments := totalWidth / segmentWidth
	segments := make([]image.Image, 0, numSegments)

	subImg := img.(subImager)

	for i := range numSegments {
		left := bounds.Min.X + i*segmentWidth
		right := left + segmentWidth
		cropRect := image.Rect(left, bounds.Min.Y, right, bounds.Max.Y)
		segments = append(segments, subImg.SubImage(cropRect))
	}

	return segments
}

func ResizeImage(img image.Image, targetW, targetH int) image.Image {
	finalImg := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	srcBounds := img.Bounds()
	draw.BiLinear.Scale(finalImg, finalImg.Bounds(), img, srcBounds, draw.Src, nil)
	return finalImg
}

func Autocrop(img image.Image) (image.Image, bool) {
	bounds := img.Bounds()

	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X, bounds.Min.Y

	foundNotEmpty := false

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()

			if a <= 0 {
				continue
			}

			foundNotEmpty = true

			if x < minX {
				minX = x
			}

			if x > maxX {
				maxX = x
			}

			if y < minY {
				minY = y
			}

			if y > maxY {
				maxY = y
			}
		}
	}

	if !foundNotEmpty {
		return nil, false
	}

	cropRect := image.Rect(minX, minY, maxX+1, maxY+1)
	return img.(subImager).SubImage(cropRect), true
}
