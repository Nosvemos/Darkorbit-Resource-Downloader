package model

type Seed struct {
	RelativePath string
	Category     string
}

type Resource struct {
	RelativePath string
	URL          string
	Category     string
	ManifestPath string
	Hash         string
	Type         string
	Core         bool
}

type DownloadItem struct {
	Resource Resource
	Reason   string
}

type Summary struct {
	SeedsFetched     int
	ManifestCount    int
	ResourceCount    int
	PlannedDownloads int
	SkippedResources int
}
