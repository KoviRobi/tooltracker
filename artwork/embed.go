package artwork

import _ "embed"

// generated with
//
//	$ magick \
//	   -density 300 \
//	   -define icon:auto-resize=64,48,32,16 \
//	   -background none \
//	   artwork/logo.svg \
//	   artwork/favicon.ico
//
//go:embed favicon.ico
var Favicon_ico []byte

//go:embed logo.svg
var Logo_svg []byte
