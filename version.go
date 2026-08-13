package main

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
}
