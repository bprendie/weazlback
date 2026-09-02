package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type rgb struct{ r, g, b float64 }

func renderGradientLogo(wordmark string, width int) string {
	logoWidth := maxLineWidth(wordmark)
	colored := gradientBlock(wordmark, gradientStops(accent, secondary, warning, success))
	if width <= 0 || width < logoWidth+10 {
		return colored
	}

	leftWidth, gapWidth := 6, 1
	rightWidth := max(4, width-logoWidth-leftWidth-gapWidth*2)
	slashes := gradientStops(warning, secondary, accent)
	lines := strings.Split(colored, "\n")
	var out strings.Builder
	for i, line := range lines {
		right := max(1, rightWidth-i)
		out.WriteString(gradientText(strings.Repeat("╱", leftWidth), slashes))
		out.WriteByte(' ')
		out.WriteString(line)
		if pad := logoWidth - lipgloss.Width(line); pad > 0 {
			out.WriteString(strings.Repeat(" ", pad))
		}
		out.WriteByte(' ')
		out.WriteString(gradientText(strings.Repeat("╱", right), slashes))
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func gradientBlock(value string, stops []rgb) string {
	width := maxLineWidth(value)
	var out strings.Builder
	for y, line := range strings.Split(value, "\n") {
		for x, r := range line {
			if r == ' ' {
				out.WriteRune(r)
				continue
			}
			t := float64(x) / float64(max(1, width-1))
			out.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(sampleGradient(t, stops))).Bold(true).Render(string(r)))
		}
		if y < len(strings.Split(value, "\n"))-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func gradientText(value string, stops []rgb) string {
	width := max(1, lipgloss.Width(value))
	var out strings.Builder
	for x, r := range value {
		t := float64(x) / float64(max(1, width-1))
		out.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(sampleGradient(t, stops))).Bold(true).Render(string(r)))
	}
	return out.String()
}

func gradientStops(colors ...lipgloss.Color) []rgb {
	result := make([]rgb, 0, len(colors))
	for _, color := range colors {
		var value int
		if _, err := fmt.Sscanf(string(color), "#%06X", &value); err == nil {
			result = append(result, rgb{float64(value >> 16 & 0xff), float64(value >> 8 & 0xff), float64(value & 0xff)})
		}
	}
	return result
}

func sampleGradient(t float64, stops []rgb) string {
	if len(stops) == 0 {
		return string(foreground)
	}
	scaled := t * float64(len(stops)-1)
	i := min(int(math.Floor(scaled)), len(stops)-1)
	if i == len(stops)-1 {
		return stops[i].hex()
	}
	local := scaled - float64(i)
	return rgb{stops[i].r + (stops[i+1].r-stops[i].r)*local,
		stops[i].g + (stops[i+1].g-stops[i].g)*local,
		stops[i].b + (stops[i+1].b-stops[i].b)*local}.hex()
}

func (c rgb) hex() string {
	return fmt.Sprintf("#%02X%02X%02X", int(math.Round(c.r)), int(math.Round(c.g)), int(math.Round(c.b)))
}

func maxLineWidth(value string) int {
	width := 1
	for _, line := range strings.Split(value, "\n") {
		width = max(width, lipgloss.Width(line))
	}
	return width
}
