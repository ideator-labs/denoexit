package shim

import _ "embed"

//go:embed shim.js
var ShimJS []byte

//go:embed entry.js
var EntryJS []byte
