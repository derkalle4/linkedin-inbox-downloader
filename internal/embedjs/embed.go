package embedjs

import _ "embed"

//go:embed list.js
var ListJS string

//go:embed open.js
var OpenJS string

//go:embed load.js
var LoadJS string

//go:embed extract.js
var ExtractJS string
