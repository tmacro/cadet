package webui

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type AssetDir struct {
	fs     fs.FS
	prefix string
}

var _ fs.FS = (*AssetDir)(nil)

func (ad *AssetDir) Open(name string) (fs.File, error) {
	path := filepath.Join(ad.prefix, name)
	fmt.Println(path)
	return ad.fs.Open(path)
}

func AssetsFromPath(path string) fs.FS {
	dir := os.DirFS(path)
	return &AssetDir{
		fs:     dir,
		prefix: "",
	}
}

func AssetsFromEmbed(prefix string, dir fs.FS) fs.FS {
	return &AssetDir{
		fs:     dir,
		prefix: prefix,
	}
}

//go:embed static/*
var static embed.FS
var StaticAssets fs.FS

//go:embed templates/*
var templates embed.FS
var TemplateAssets fs.FS
var EmbeddedTemplates TemplateCache

func init() {
	StaticAssets = AssetsFromEmbed("static", static)
	TemplateAssets = AssetsFromEmbed("templates", templates)
	EmbeddedTemplates = NewTemplateCache(TemplateAssets, Cache)
}
