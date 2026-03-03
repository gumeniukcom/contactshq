package web

import "embed"

//go:embed all:static/spa
var SPAFiles embed.FS

//go:embed templates/*
var TemplateFiles embed.FS
