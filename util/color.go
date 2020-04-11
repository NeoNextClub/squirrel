package util

import "strconv"

const (
	// common
	reset  = "\033[0m"
	normal = 0
	bold   = 1

	// special
	dim       = 2
	underline = 4
	blink     = 5
	reverse   = 7
	hidden    = 8

	// colors
	black       = 30
	red         = 31
	green       = 32
	yellow      = 33
	blue        = 34
	purple      = 35
	cyan        = 36
	lightGray   = 37
	darkGray    = 90
	lightRed    = 91
	lightGreen  = 92
	lightYellow = 93
	lightBlue   = 94
	lightPurple = 95
	lightCyan   = 96
	white       = 97
)

// Render rends text with parameters
func Render(colorCode int, fontSize int, content string) string {
	return "\033[" + strconv.Itoa(fontSize) + ";" + strconv.Itoa(colorCode) + "m" + content + reset
}

// Red text
func Red(txt string) string {
	return Render(red, normal, txt)
}

// BRed returns bold red test
func BRed(txt string) string {
	return Render(red, bold, txt)
}
