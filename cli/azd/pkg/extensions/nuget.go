package extensions

type VersionInfo struct {
	ID        string `json:"@id"`
	Downloads int    `json:"downloads"`
	Version   string `json:"version"`
}

type PackageInfo struct {
	Id           string        `json:"id"`
	Version      string        `json:"version"`
	Description  string        `json:"description"`
	Versions     []VersionInfo `json:"versions"`
	Authors      []string      `json:"authors"`
	IconURL      string        `json:"iconUrl"`
	LicenseURL   string        `json:"licenseUrl"`
	ProjectURL   *string       `json:"projectUrl"`
	Registration string        `json:"registration"`
	Summary      string        `json:"summary"`
	Tags         []string      `json:"tags"`
	Title        string        `json:"title"`
}
