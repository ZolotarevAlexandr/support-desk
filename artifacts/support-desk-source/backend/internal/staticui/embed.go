//go:build !production

package staticui

import (
	"embed"
	"io/fs"
)

//go:embed dev
var files embed.FS

func Assets() fs.FS { assets, _ := fs.Sub(files, "dev"); return assets }
