package models

// types for translations
type ImageConstraints struct {
	AppType       string
	Version       string
	DatabaseTypes map[string]struct{}
}
