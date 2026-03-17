package manifest

import (
	"encoding/xml"
	"os"
	"path"
	"strings"

	"darkorbit-resource-downloader/internal/model"
)

type FileCollection struct {
	XMLName   xml.Name       `xml:"filecollection"`
	Locations []LocationNode `xml:"location"`
	Files     []FileNode     `xml:"file"`
}

type LocationNode struct {
	ID   string `xml:"id,attr"`
	Path string `xml:"path,attr"`
}

type FileNode struct {
	ID       string `xml:"id,attr"`
	Location string `xml:"location,attr"`
	Name     string `xml:"name,attr"`
	Type     string `xml:"type,attr"`
	Hash     string `xml:"hash,attr"`
}

func LoadResourcesFromFile(localPath, manifestRelPath, baseURL, category string) ([]model.Resource, bool, error) {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return nil, false, err
	}

	var collection FileCollection
	if err := xml.Unmarshal(data, &collection); err != nil {
		return nil, false, nil
	}
	if collection.XMLName.Local != "filecollection" {
		return nil, false, nil
	}

	locationMap := make(map[string]string, len(collection.Locations))
	for _, location := range collection.Locations {
		locationMap[location.ID] = location.Path
	}

	basePrefix := collectionBasePrefix(manifestRelPath)
	resources := make([]model.Resource, 0, len(collection.Files))
	for _, file := range collection.Files {
		locationPath := locationMap[file.Location]
		if locationPath == "" && file.Location != "" {
			locationPath = filepathToSlash(file.Location)
			if !strings.HasSuffix(locationPath, "/") {
				locationPath += "/"
			}
		}
		relativePath := path.Clean(basePrefix + locationPath + file.Name + "." + file.Type)
		if relativePath == "." {
			continue
		}
		resources = append(resources, model.Resource{
			RelativePath: relativePath,
			URL:          joinURL(baseURL, relativePath, file.Hash),
			Category:     category,
			ManifestPath: manifestRelPath,
			Hash:         file.Hash,
			Type:         file.Type,
		})
	}
	return resources, true, nil
}

func collectionBasePrefix(manifestRelPath string) string {
	dir := path.Dir(filepathToSlash(manifestRelPath))
	if path.Base(dir) == "xml" {
		dir = path.Dir(dir)
	}
	if dir == "." {
		return ""
	}
	return strings.TrimSuffix(dir, "/") + "/"
}

func filepathToSlash(in string) string {
	return strings.ReplaceAll(in, "\\", "/")
}

func joinURL(baseURL, relativePath, hash string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	url := baseURL + "/" + strings.TrimLeft(relativePath, "/")
	if hash != "" {
		url += "?__cv=" + hash
	}
	return url
}
