package main

import "math"

type vertex struct{ x, y float64 }

func segmentDistance(x, y float64, a, b vertex) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	t := ((x-a.x)*dx + (y-a.y)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(x-a.x-t*dx, y-a.y-t*dy)
}

func renderIcon(size int, state connectionState) []byte {
	pixels := make([]byte, size*size*4)
	const samples = 8
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var red, green, blue, alpha float64
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/samples) * 16 / float64(size)
					y := (float64(py) + (float64(sy)+0.5)/samples) * 16 / float64(size)
					r, g, b, a := 0.0, 0.0, 0.0, 0.0
					cx, cy := (x-7.8)/1.1+7.8, (y-6.7)/1.1+6.7
					cloud := math.Hypot(cx-4, cy-8.2) < 3 ||
						math.Hypot(cx-7.2, cy-5.9) < 3.7 ||
						math.Hypot(cx-11.5, cy-7.8) < 3.1 ||
						(cx >= 4 && cx <= 11.5 && cy >= 6.5 && cy <= 11.2)
					if cloud {
						r, g, b, a = 255, 255, 255, 255
					}
					{
						distance := math.Hypot(x-11.9, y-11.8)
						if distance < 4.1 {
							r, g, b, a = 0, 0, 0, 0
						}
						if distance < 3.45 {
							r, g, b, a = 224, 55, 65, 255
							if state == connected {
								r, g, b = 35, 177, 85
							} else if state != disconnected {
								r, g, b = 255, 196, 0
							}
							mark := false
							if state == connected {
								mark = segmentDistance(x, y, vertex{9.9, 11.8}, vertex{11.3, 13.1}) < 0.6 || segmentDistance(x, y, vertex{11.3, 13.1}, vertex{14, 10.4}) < 0.6
							} else if state == disconnected {
								mark = segmentDistance(x, y, vertex{10.5, 10.4}, vertex{13.3, 13.2}) < 0.6 || segmentDistance(x, y, vertex{13.3, 10.4}, vertex{10.5, 13.2}) < 0.6
							} else {
								mark = segmentDistance(x, y, vertex{11.9, 9.8}, vertex{11.9, 12}) < 0.65 || math.Hypot(x-11.9, y-13.7) < 0.65
							}
							if mark {
								r, g, b = 255, 255, 255
								if state != connected && state != disconnected {
									r, g, b = 35, 35, 35
								}
							}
						}
					}
					red += r
					green += g
					blue += b
					alpha += a
				}
			}
			// CreateIconIndirect expects premultiplied BGRA pixels.
			i := (py*size + px) * 4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = byte(blue/(samples*samples)), byte(green/(samples*samples)), byte(red/(samples*samples)), byte(alpha/(samples*samples))
		}
	}
	return pixels
}
