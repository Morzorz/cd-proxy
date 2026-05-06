//go:build darwin

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

var runningIcon []byte
var stoppedIcon []byte

func init() {
	runningIcon = generateIcon(255)
	stoppedIcon = generateIcon(100)
}

func generateIcon(alpha uint8) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 22, 22))

	// Draw a simple "C" shape or circular node
	centerX, centerY := 11, 11
	radius := 7

	for y := 0; y < 22; y++ {
		for x := 0; x < 22; x++ {
			dx := x - centerX
			dy := y - centerY
			dist := dx*dx + dy*dy

			// Outer circle
			if dist <= radius*radius && dist >= (radius-2)*(radius-2) {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: alpha})
			}
			// Arrow heads (left and right)
			if dist < 4 && ((dx > 2 && dy > -1 && dy < 1) || (dx < -2 && dy > -1 && dy < 1)) {
				img.Set(x, y, color.RGBA{R: 0, G: 0, B: 0, A: alpha})
			}
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
