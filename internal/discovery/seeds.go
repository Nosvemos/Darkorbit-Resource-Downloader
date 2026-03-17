package discovery

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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
	{RelativePath: "unityApi/events/spaceSearch.php", Category: "unityApi"},
	{RelativePath: "unityApi/events/ssRewards.php", Category: "unityApi"},
	{RelativePath: "flashAPI/brNews.php", Category: "flashAPI"},
	{RelativePath: "flashAPI/dailyLogin.php", Category: "flashAPI"},
	{RelativePath: "flashAPI/loadingScreen.php", Category: "flashAPI"},
	{RelativePath: "resources/resource_command_center.xml", Category: "resources"},
	{RelativePath: "resources/command-center/positions.xml", Category: "resources"},
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
	case strings.HasPrefix(rel, "spacemap/templates/"):
		return strings.HasPrefix(base, "language_") && strings.HasSuffix(base, ".xml")
	case strings.HasPrefix(rel, "do_img/global/xml/") && strings.HasSuffix(rel, ".xml"):
		return true
	case strings.HasPrefix(rel, "do_img/") && strings.HasSuffix(rel, "/xml/resource_localized.xml"):
		return true
	case strings.HasPrefix(rel, "spacemap/xml/") && strings.HasSuffix(rel, ".xml"):
		return true
	case strings.HasPrefix(rel, "resources/") && strings.HasSuffix(rel, ".xml"):
		return true
	case strings.HasPrefix(rel, "unityApi/events/") && strings.HasSuffix(rel, ".xml"):
		return false
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

func AddLanguageBootstrapSeeds(outputDir string, seeds []model.Seed, languages map[string]bool) []model.Seed {
	seen := make(map[string]model.Seed, len(seeds))
	for _, seed := range seeds {
		seen[seed.RelativePath] = seed
	}

	for _, lang := range resolveBootstrapLanguages(outputDir, seeds, languages) {
		seen["spacemap/templates/language_"+lang+".xml"] = model.Seed{
			RelativePath: "spacemap/templates/language_" + lang + ".xml",
			Category:     "templates",
		}
		seen["do_img/"+lang+"/xml/resource_localized.xml"] = model.Seed{
			RelativePath: "do_img/" + lang + "/xml/resource_localized.xml",
			Category:     "do_img",
		}
	}

	merged := make([]model.Seed, 0, len(seen))
	for _, seed := range seen {
		merged = append(merged, seed)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].RelativePath < merged[j].RelativePath
	})
	return merged
}

func ResolveBootstrapLanguages(outputDir string, seeds []model.Seed, languages map[string]bool) []string {
	return resolveBootstrapLanguages(outputDir, seeds, languages)
}

func resolveBootstrapLanguages(outputDir string, seeds []model.Seed, languages map[string]bool) []string {
	if len(languages) == 0 {
		return []string{"en"}
	}

	langs := map[string]bool{}
	if languages["all"] {
		for _, lang := range discoverExistingLanguages(outputDir, seeds) {
			langs[lang] = true
		}
		if len(langs) == 0 {
			langs["en"] = true
		}
	} else {
		for lang := range languages {
			if lang == "" || lang == "all" {
				continue
			}
			langs[lang] = true
		}
	}

	resolved := make([]string, 0, len(langs))
	for lang := range langs {
		resolved = append(resolved, lang)
	}
	sort.Strings(resolved)
	return resolved
}

func DiscoverLiveLanguages(ctx context.Context, baseURL string) ([]string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	return discoverLiveLanguages(ctx, client, strings.TrimRight(baseURL, "/"))
}

func discoverLiveLanguages(ctx context.Context, client *http.Client, baseURL string) ([]string, error) {
	html, err := fetchText(ctx, client, baseURL)
	if err != nil {
		return nil, err
	}

	candidates := extractLanguagesFromHTML(html)
	available := make([]string, 0, len(candidates))
	for _, lang := range candidates {
		ok, err := languageBootstrapExists(ctx, client, baseURL, lang)
		if err != nil {
			return nil, err
		}
		if ok {
			available = append(available, lang)
		}
	}
	return available, nil
}

func discoverExistingLanguages(outputDir string, seeds []model.Seed) []string {
	langs := map[string]bool{}
	for _, seed := range seeds {
		if lang, ok := languageFromSeedPath(seed.RelativePath); ok {
			langs[CanonicalLanguageCode(lang)] = true
		}
	}

	patterns := []string{
		filepath.Join(outputDir, "spacemap", "templates", "language_*.xml"),
		filepath.Join(outputDir, "do_img", "*", "xml", "resource_localized.xml"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			rel, err := filepath.Rel(outputDir, match)
			if err != nil {
				continue
			}
			if lang, ok := languageFromSeedPath(filepath.ToSlash(rel)); ok {
				langs[CanonicalLanguageCode(lang)] = true
			}
		}
	}

	resolved := make([]string, 0, len(langs))
	for lang := range langs {
		resolved = append(resolved, lang)
	}
	sort.Strings(resolved)
	return resolved
}

func languageFromSeedPath(rel string) (string, bool) {
	rel = filepath.ToSlash(rel)

	if strings.HasPrefix(rel, "spacemap/templates/language_") && strings.HasSuffix(rel, ".xml") {
		lang := CanonicalLanguageCode(strings.TrimSuffix(strings.TrimPrefix(rel, "spacemap/templates/language_"), ".xml"))
		if lang != "" {
			return lang, true
		}
	}

	if strings.HasPrefix(rel, "do_img/") && strings.HasSuffix(rel, "/xml/resource_localized.xml") {
		parts := strings.Split(rel, "/")
		if len(parts) >= 4 && parts[1] != "" && parts[1] != "global" {
			return CanonicalLanguageCode(parts[1]), true
		}
	}

	return "", false
}

func extractLanguagesFromHTML(html string) []string {
	re := regexp.MustCompile(`lang=([A-Za-z_]+)`)
	matches := re.FindAllStringSubmatch(html, -1)
	seen := map[string]bool{}
	langs := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		lang := CanonicalLanguageCode(match[1])
		if lang == "" || lang == "all" || seen[lang] {
			continue
		}
		seen[lang] = true
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

func languageBootstrapExists(ctx context.Context, client *http.Client, baseURL, lang string) (bool, error) {
	paths := []string{
		baseURL + "/spacemap/templates/language_" + lang + ".xml",
		baseURL + "/do_img/" + lang + "/xml/resource_localized.xml",
	}
	for _, target := range paths {
		ok, err := urlExists(ctx, client, target)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func urlExists(ctx context.Context, client *http.Client, target string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}
	return false, nil
}

func fetchText(ctx context.Context, client *http.Client, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &httpStatusError{statusCode: resp.StatusCode, target: target}
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type httpStatusError struct {
	statusCode int
	target     string
}

func (e *httpStatusError) Error() string {
	return "unexpected status " + strconv.Itoa(e.statusCode) + " for " + e.target
}

func CanonicalLanguageCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if strings.EqualFold(code, "all") {
		return "all"
	}

	code = strings.ReplaceAll(code, "-", "_")
	parts := strings.Split(code, "_")
	for i := range parts {
		if parts[i] == "" {
			return ""
		}
		if i == 0 {
			parts[i] = strings.ToLower(parts[i])
			continue
		}
		parts[i] = strings.ToUpper(parts[i])
	}
	return strings.Join(parts, "_")
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
		lang := CanonicalLanguageCode(strings.TrimSuffix(strings.TrimPrefix(rel, "spacemap/templates/language_"), ".xml"))
		return languages[lang]
	}

	if strings.HasPrefix(rel, "spacemap/templates/") {
		parts := strings.Split(rel, "/")
		if len(parts) >= 3 {
			return languages[CanonicalLanguageCode(parts[2])]
		}
	}

	if strings.HasPrefix(rel, "do_img/") {
		parts := strings.Split(rel, "/")
		if len(parts) >= 2 {
			lang := CanonicalLanguageCode(parts[1])
			if lang == "global" {
				return true
			}
			return languages[lang]
		}
	}

	return true
}
