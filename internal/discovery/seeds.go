package discovery

import (
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"darkorbit-resource-downloader/internal/model"
)

var hardcodedSeeds = []model.Seed{
	{RelativePath: "crossdomain.xml", Category: "core"},
	{RelativePath: "spacemap/preloader.swf", Category: "core"},
	{RelativePath: "spacemap/loadingscreen.swf", Category: "core"},
	{RelativePath: "spacemap/main.swf", Category: "core"},
	{RelativePath: "spacemap/xml/maps.php", Category: "core"},
	{RelativePath: "spacemap/xml/profile.xml", Category: "spacemap"},
	{RelativePath: "spacemap/xml/resources.xml", Category: "spacemap"},
	{RelativePath: "spacemap/xml/resources_3d.xml", Category: "spacemap"},
	{RelativePath: "spacemap/xml/resources_3d_particles.xml", Category: "spacemap"},
	{RelativePath: "spacemap/xml/assets_loadingScreen.xml", Category: "spacemap"},
	{RelativePath: "do_img/global/xml/resource_items.xml", Category: "do_img"},
	{RelativePath: "do_img/global/xml/resource_events.xml", Category: "do_img"},
	{RelativePath: "do_img/global/xml/resource_achievements.xml", Category: "do_img"},
	{RelativePath: "do_img/global/xml/resource_jumpgate.xml", Category: "do_img"},
	{RelativePath: "do_img/en/xml/resource_localized.xml", Category: "do_img"},
	{RelativePath: "do_img/es/xml/resource_localized.xml", Category: "do_img"},
	{RelativePath: "unityApi/events/spaceSearch.php", Category: "unityApi"},
	{RelativePath: "unityApi/events/ssRewards.php", Category: "unityApi"},
	{RelativePath: "flashAPI/brNews.php", Category: "flashAPI"},
	{RelativePath: "flashAPI/dailyLogin.php", Category: "flashAPI"},
	{RelativePath: "flashAPI/loadingScreen.php", Category: "flashAPI"},
	{RelativePath: "resources/resource_command_center.xml", Category: "resources"},
	{RelativePath: "resources/command-center/positions.xml", Category: "resources"},
	{RelativePath: "spacemap/templates/language_en.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/language_es.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/language_tr.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/flashres.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_achievement.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_eic.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_faq.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_galaxyGates.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_inc.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_inventory.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_items.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_loadingScreen.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_news.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/en/resource_quest.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/es/flashres.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/es/resource_achievement.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/es/resource_items.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/es/resource_loadingScreen.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/es/resource_quest.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/flashres.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_achievement.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_chat.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_eic.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_faq.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_galaxyGates.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_inc.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_inventory.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_items.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_loadingScreen.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_news.xml", Category: "templates"},
	{RelativePath: "spacemap/templates/tr/resource_quest.xml", Category: "templates"},
}

var scanRoots = []string{
	"spacemap/xml",
	"spacemap/templates",
	"do_img",
	"unityApi",
	"flashAPI",
	"resources",
}

func DiscoverSeeds(outputDir string) ([]model.Seed, error) {
	seen := map[string]model.Seed{}
	for _, seed := range hardcodedSeeds {
		seen[seed.RelativePath] = seed
	}

	for _, relRoot := range scanRoots {
		root := filepath.Join(outputDir, filepath.FromSlash(relRoot))
		err := filepath.WalkDir(root, func(current string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if current == root {
					return nil
				}
				return walkErr
			}
			if d.IsDir() {
				if strings.Contains(filepath.ToSlash(current), "spacemap_decompiled") {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(outputDir, current)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !isSeedCandidate(rel) {
				return nil
			}
			if _, ok := seen[rel]; !ok {
				seen[rel] = model.Seed{
					RelativePath: rel,
					Category:     categoryFor(rel),
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	seeds := make([]model.Seed, 0, len(seen))
	for _, seed := range seen {
		seeds = append(seeds, seed)
	}
	sort.Slice(seeds, func(i, j int) bool {
		return seeds[i].RelativePath < seeds[j].RelativePath
	})
	return seeds, nil
}

func categoryFor(rel string) string {
	switch {
	case rel == "crossdomain.xml" || strings.HasSuffix(rel, ".swf") || strings.HasSuffix(rel, "maps.php"):
		return "core"
	case strings.HasPrefix(rel, "spacemap/templates/"):
		return "templates"
	case strings.HasPrefix(rel, "spacemap/"):
		return "spacemap"
	case strings.HasPrefix(rel, "do_img/"):
		return "do_img"
	case strings.HasPrefix(rel, "unityApi/"):
		return "unityApi"
	case strings.HasPrefix(rel, "flashAPI/"):
		return "flashAPI"
	case strings.HasPrefix(rel, "resources/"):
		return "resources"
	default:
		return "other"
	}
}

func isSeedCandidate(rel string) bool {
	if strings.Contains(rel, "spacemap_decompiled") {
		return false
	}
	base := path.Base(rel)
	if base == "index.php" {
		return false
	}
	switch {
	case strings.HasPrefix(rel, "unityApi/events/") && strings.HasSuffix(rel, ".xml"):
		return false
	case strings.HasSuffix(rel, ".xml"):
		return true
	case strings.HasPrefix(rel, "unityApi/events/") && strings.HasSuffix(rel, ".php"):
		return true
	case strings.HasPrefix(rel, "flashAPI/") && strings.HasSuffix(rel, ".php"):
		return true
	case rel == "spacemap/xml/maps.php":
		return true
	default:
		return false
	}
}

func FilterSeeds(seeds []model.Seed, allowed map[string]bool, includeCore bool) []model.Seed {
	if len(allowed) == 0 {
		return seeds
	}
	filtered := make([]model.Seed, 0, len(seeds))
	for _, seed := range seeds {
		if seed.Category == "core" && !includeCore {
			continue
		}
		if allowed[seed.Category] || allowed["all"] || (seed.Category == "core" && allowed["core"]) {
			filtered = append(filtered, seed)
		}
	}
	return filtered
}

func MatchesLanguagePath(rel string, languages map[string]bool) bool {
	if len(languages) == 0 || languages["all"] {
		return true
	}

	rel = filepath.ToSlash(rel)

	if strings.HasPrefix(rel, "spacemap/templates/language_") && strings.HasSuffix(rel, ".xml") {
		lang := strings.TrimSuffix(strings.TrimPrefix(rel, "spacemap/templates/language_"), ".xml")
		return languages[lang]
	}

	if strings.HasPrefix(rel, "spacemap/templates/") {
		parts := strings.Split(rel, "/")
		if len(parts) >= 3 {
			return languages[parts[2]]
		}
	}

	if strings.HasPrefix(rel, "do_img/") {
		parts := strings.Split(rel, "/")
		if len(parts) >= 2 {
			lang := parts[1]
			if lang == "global" {
				return true
			}
			return languages[lang]
		}
	}

	return true
}
