package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"darkorbit-resource-downloader/internal/discovery"
	"darkorbit-resource-downloader/internal/downloader"
	"darkorbit-resource-downloader/internal/integrity"
	"darkorbit-resource-downloader/internal/manifest"
	"darkorbit-resource-downloader/internal/model"
	"darkorbit-resource-downloader/internal/state"
	"github.com/charmbracelet/lipgloss"
)

type config struct {
	BaseURL             string
	OutputDir           string
	Concurrency         int
	MinConcurrency      int
	AutoTuneConcurrency bool
	RequestInterval     time.Duration
	Force               bool
	Categories          map[string]bool
	Languages           map[string]bool
	LogFile             string
}

type printer struct {
	stdout io.Writer
	file   *os.File
}

func newPrinter(logFile string) (*printer, error) {
	p := &printer{stdout: os.Stdout}
	if strings.TrimSpace(logFile) == "" {
		return p, nil
	}

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	p.file = file
	return p, nil
}

func (p *printer) Close() error {
	if p.file != nil {
		return p.file.Close()
	}
	return nil
}

func (p *printer) Printf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	fmt.Fprint(p.stdout, message)
	if p.file != nil {
		fmt.Fprint(p.file, message)
	}
}

func (p *printer) Println(args ...any) {
	message := fmt.Sprintln(args...)
	fmt.Fprint(p.stdout, message)
	if p.file != nil {
		fmt.Fprint(p.file, message)
	}
}

func (p *printer) Title(title string) {
	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	p.writeStyled(style.Render(title), title)
}

func (p *printer) Section(title string) {
	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	p.writeStyled(style.Render(title), title)
}

func (p *printer) Status(label, message string) {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	switch label {
	case "OK":
		style = style.Foreground(lipgloss.Color("42"))
	case "FAIL":
		style = style.Foreground(lipgloss.Color("196"))
	case "FETCH":
		style = style.Foreground(lipgloss.Color("81"))
	case "DOWNLOAD":
		style = style.Foreground(lipgloss.Color("220"))
	default:
		style = style.Foreground(lipgloss.Color("250"))
	}
	styled := fmt.Sprintf("%s %s", style.Render(label), message)
	plain := fmt.Sprintf("%s %s", label, message)
	p.writeStyled(styled, plain)
}

func (p *printer) KV(key string, value any) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styled := fmt.Sprintf("%s %v", style.Render(key+":"), value)
	plain := fmt.Sprintf("%s: %v", key, value)
	p.writeStyled(styled, plain)
}

func (p *printer) writeStyled(styled, plain string) {
	fmt.Fprintln(p.stdout, styled)
	if p.file != nil {
		fmt.Fprintln(p.file, plain)
	}
}

func Run(ctx context.Context, args []string) error {
	command := "sync"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}

	cfg, err := parseConfig(command, args)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return err
	}

	logPath := cfg.LogFile
	if logPath != "" && !filepath.IsAbs(logPath) {
		logPath = filepath.Clean(logPath)
	}
	out := os.Stdout
	_ = out

	pr, err := newPrinter(logPath)
	if err != nil {
		return err
	}
	defer pr.Close()

	switch command {
	case "sync":
		return runSync(ctx, cfg, pr)
	case "plan":
		return runPlan(ctx, cfg, pr)
	case "fetch-manifests":
		return runFetchManifests(ctx, cfg, pr)
	case "verify":
		return runVerify(cfg, pr)
	case "help", "-h", "--help":
		printUsage(pr)
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func parseConfig(command string, args []string) (config, error) {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	baseURL := fs.String("base-url", "https://www.darkorbit.com", "Base URL to fetch resources from")
	outputDir := fs.String("output", "darkorbit-files", "Output root for the mirrored files")
	concurrency := fs.Int("concurrency", 8, "Number of parallel downloads")
	minConcurrency := fs.Int("min-concurrency", 1, "Lowest concurrency the auto-tuner may reduce to after repeated 429/503 responses")
	autoTuneConcurrency := fs.Bool("auto-tune-concurrency", true, "Automatically lower and re-raise concurrency at runtime based on throttle responses")
	requestInterval := fs.Duration("request-interval", 250*time.Millisecond, "Minimum delay between starting HTTP requests, e.g. 250ms or 1s")
	force := fs.Bool("force", false, "Redownload files even if they already exist")
	categoryCSV := fs.String("category", "all", "Comma-separated categories: all, spacemap, do_img, core, templates, unityApi, flashAPI, resources")
	languageCSV := fs.String("languages", "en", "Comma-separated languages for localized/template assets, e.g. en,de,au or all")
	logFile := fs.String("log-file", "app.log", "Optional log file path, empty disables file logging")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	return config{
		BaseURL:             strings.TrimRight(*baseURL, "/"),
		OutputDir:           *outputDir,
		Concurrency:         *concurrency,
		MinConcurrency:      *minConcurrency,
		AutoTuneConcurrency: *autoTuneConcurrency,
		RequestInterval:     *requestInterval,
		Force:               *force,
		Categories:          parseCategories(*categoryCSV),
		Languages:           parseLanguages(*languageCSV),
		LogFile:             *logFile,
	}, nil
}

func runSync(ctx context.Context, cfg config, pr *printer) error {
	dl := newDownloader(cfg)
	preparedCfg, seeds, err := prepareSeeds(ctx, cfg, pr, true)
	if err != nil {
		return err
	}
	cfg = preparedCfg

	if err := fetchSeeds(ctx, dl, cfg, seeds, true, pr); err != nil {
		return err
	}

	resources, manifestsParsed, err := loadResources(cfg, seeds)
	if err != nil {
		return err
	}
	plan, skipped, st, err := BuildDownloadPlan(cfg.OutputDir, resources, cfg.Force)
	if err != nil {
		return err
	}

	printSummary("sync", model.Summary{
		SeedsFetched:     countFetchedSeeds(seeds, cfg.Categories, cfg.Languages, true),
		ManifestCount:    manifestsParsed,
		ResourceCount:    len(resources),
		PlannedDownloads: len(plan),
		SkippedResources: skipped,
	}, pr)
	printCategoryBreakdownResources(resources, "Resources by category", pr)
	printCategoryBreakdownPlan(plan, "Planned downloads by category", pr)
	printReasonBreakdown(plan, pr)

	if len(plan) == 0 {
		pr.Println("No resource downloads needed.")
		return state.Save(cfg.OutputDir, st)
	}

	results := dl.DownloadAll(ctx, cfg.OutputDir, plan, cfg.Concurrency)
	var completed, failed int
	total := len(plan)
	lastEffective := -1
	for result := range results {
		stats := dl.Stats()
		if stats.AutoTuneEnabled && stats.EffectiveConcurrency != lastEffective {
			pr.Status("TUNE", fmt.Sprintf("effective concurrency %d/%d (min %d, in-flight %d)", stats.EffectiveConcurrency, stats.MaxConcurrency, stats.MinConcurrency, stats.InFlight))
			lastEffective = stats.EffectiveConcurrency
		}
		live := fmt.Sprintf("limit %d/%d, in-flight %d", stats.EffectiveConcurrency, stats.MaxConcurrency, stats.InFlight)
		if result.Err != nil {
			failed++
			pr.Status("FAIL", fmt.Sprintf("[%d/%d] %s (%s, %s): %v", completed+failed, total, result.Item.Resource.RelativePath, result.Item.Reason, live, result.Err))
			continue
		}
		completed++
		pr.Status("OK", fmt.Sprintf("[%d/%d] %s (%s, %s)", completed+failed, total, result.Item.Resource.RelativePath, result.Item.Reason, live))
		st.Resources[result.Item.Resource.RelativePath] = state.StateEntry{
			Hash: result.Item.Resource.Hash,
			URL:  result.Item.Resource.URL,
		}
	}

	if err := state.Save(cfg.OutputDir, st); err != nil {
		return err
	}
	pr.Printf("Completed: %d, Failed: %d\n", completed, failed)
	if failed > 0 {
		return errors.New("some downloads failed")
	}
	return nil
}

func runPlan(ctx context.Context, cfg config, pr *printer) error {
	dl := newDownloader(cfg)
	preparedCfg, seeds, err := prepareSeeds(ctx, cfg, pr, true)
	if err != nil {
		return err
	}
	cfg = preparedCfg
	if err := fetchSeeds(ctx, dl, cfg, seeds, false, pr); err != nil {
		return err
	}

	resources, manifestsParsed, err := loadResources(cfg, seeds)
	if err != nil {
		return err
	}
	plan, skipped, _, err := BuildDownloadPlan(cfg.OutputDir, resources, cfg.Force)
	if err != nil {
		return err
	}

	printSummary("plan", model.Summary{
		SeedsFetched:     countFetchedSeeds(seeds, cfg.Categories, cfg.Languages, false),
		ManifestCount:    manifestsParsed,
		ResourceCount:    len(resources),
		PlannedDownloads: len(plan),
		SkippedResources: skipped,
	}, pr)
	printCategoryBreakdownResources(resources, "Resources by category", pr)
	printCategoryBreakdownPlan(plan, "Planned downloads by category", pr)
	printReasonBreakdown(plan, pr)

	for _, item := range sampleItems(plan, 25) {
		pr.Printf("DOWNLOAD %s (%s)\n", item.Resource.RelativePath, item.Reason)
	}
	if len(plan) > 25 {
		pr.Printf("... and %d more\n", len(plan)-25)
	}
	return nil
}

func runFetchManifests(ctx context.Context, cfg config, pr *printer) error {
	dl := newDownloader(cfg)
	preparedCfg, seeds, err := prepareSeeds(ctx, cfg, pr, true)
	if err != nil {
		return err
	}
	cfg = preparedCfg
	if err := fetchSeeds(ctx, dl, cfg, seeds, false, pr); err != nil {
		return err
	}
	pr.Printf("Fetched %d manifest/template/core-metadata files into %s\n", countFetchedSeeds(seeds, cfg.Categories, cfg.Languages, false), cfg.OutputDir)
	return nil
}

func runVerify(cfg config, pr *printer) error {
	preparedCfg, seeds, err := prepareSeeds(context.Background(), cfg, pr, false)
	if err != nil {
		return err
	}
	cfg = preparedCfg

	seedMissing := make([]string, 0)
	for _, seed := range seeds {
		if !matchesCategory(seed.Category, cfg.Categories) {
			continue
		}
		if !discovery.MatchesLanguagePath(seed.RelativePath, cfg.Languages) {
			continue
		}
		localPath := filepath.Join(cfg.OutputDir, filepath.FromSlash(seed.RelativePath))
		if _, err := os.Stat(localPath); err != nil {
			seedMissing = append(seedMissing, seed.RelativePath)
		}
	}

	resources, manifestsParsed, err := loadResources(cfg, seeds)
	if err != nil {
		return err
	}
	resourceMissing, resourceMismatch, err := verifyResources(cfg.OutputDir, resources)
	if err != nil {
		return err
	}

	pr.Println("Verify summary")
	pr.Printf("Seed files checked: %d\n", countFetchedSeeds(seeds, cfg.Categories, cfg.Languages, true))
	pr.Printf("Filecollection manifests parsed: %d\n", manifestsParsed)
	pr.Printf("Manifest-derived resources checked: %d\n", len(resources))
	pr.Printf("Missing seed files: %d\n", len(seedMissing))
	pr.Printf("Missing resources: %d\n", len(resourceMissing))
	pr.Printf("Hash mismatches: %d\n", len(resourceMismatch))

	printGroupedPaths(resourceMissing, "Top missing groups", pr)
	printGroupedPaths(resourceMismatch, "Top hash mismatch groups", pr)
	printCategoryBreakdownResources(resources, "Resources by category", pr)
	printSample("Missing seed files", seedMissing, 20, pr)
	printSample("Missing resources", resourceMissing, 30, pr)
	printSample("Hash mismatches", resourceMismatch, 30, pr)

	if len(seedMissing)+len(resourceMissing)+len(resourceMismatch) > 0 {
		return errors.New("verification found missing or mismatched files")
	}
	return nil
}

func fetchSeeds(ctx context.Context, dl *downloader.Client, cfg config, seeds []model.Seed, includeCoreBinary bool, pr *printer) error {
	filtered := discovery.FilterSeeds(seeds, cfg.Categories, includeCoreBinary)
	for _, seed := range filtered {
		if !includeCoreBinary && (strings.HasSuffix(seed.RelativePath, ".swf") || seed.RelativePath == "crossdomain.xml") {
			continue
		}
		if !discovery.MatchesLanguagePath(seed.RelativePath, cfg.Languages) {
			continue
		}
		url := joinSeedURL(cfg.BaseURL, seed.RelativePath)
		dest := filepath.Join(cfg.OutputDir, filepath.FromSlash(seed.RelativePath))
		if err := dl.FetchToFile(ctx, url, dest); err != nil {
			var statusErr *downloader.HTTPStatusError
			if errors.As(err, &statusErr) && statusErr.StatusCode == 404 && discovery.IsOptionalSeed(seed) {
				pr.Status("SKIP", fmt.Sprintf("%s (optional seed missing on this site)", seed.RelativePath))
				continue
			}
			return fmt.Errorf("fetch seed %s: %w", seed.RelativePath, err)
		}
		pr.Status("FETCH", seed.RelativePath)
	}
	return nil
}

func loadResources(cfg config, seeds []model.Seed) ([]model.Resource, int, error) {
	resourceMap := map[string]model.Resource{}
	manifestsParsed := 0

	for _, seed := range seeds {
		if !matchesCategory(seed.Category, cfg.Categories) {
			continue
		}
		if !discovery.MatchesLanguagePath(seed.RelativePath, cfg.Languages) {
			continue
		}
		localPath := filepath.Join(cfg.OutputDir, filepath.FromSlash(seed.RelativePath))
		resources, ok, err := manifest.LoadResourcesFromFile(localPath, seed.RelativePath, cfg.BaseURL, seed.Category)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, 0, err
		}
		if !ok {
			continue
		}
		manifestsParsed++
		for _, resource := range resources {
			if !matchesCategory(resource.Category, cfg.Categories) {
				continue
			}
			if !discovery.MatchesLanguagePath(resource.RelativePath, cfg.Languages) {
				continue
			}
			if existing, exists := resourceMap[resource.RelativePath]; exists {
				if existing.Hash == "" && resource.Hash != "" {
					resourceMap[resource.RelativePath] = resource
				}
				continue
			}
			resourceMap[resource.RelativePath] = resource
		}
	}

	resources := make([]model.Resource, 0, len(resourceMap))
	for _, resource := range resourceMap {
		resources = append(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].RelativePath < resources[j].RelativePath
	})
	return resources, manifestsParsed, nil
}

func BuildDownloadPlan(outputDir string, resources []model.Resource, force bool) ([]model.DownloadItem, int, *state.State, error) {
	st, err := state.Load(outputDir)
	if err != nil {
		return nil, 0, nil, err
	}

	items := make([]model.DownloadItem, 0)
	skipped := 0
	for _, resource := range resources {
		localPath := filepath.Join(outputDir, filepath.FromSlash(resource.RelativePath))
		reason := ""

		if force {
			reason = "forced"
		} else if _, err := os.Stat(localPath); err != nil {
			reason = "missing"
		} else if resource.Hash != "" {
			match, err := integrity.FileMatchesManifestHash(localPath, resource.Hash)
			if err != nil {
				return nil, 0, nil, err
			}
			if !match {
				reason = "hash-mismatch"
			} else {
				st.Resources[resource.RelativePath] = state.StateEntry{
					Hash: resource.Hash,
					URL:  resource.URL,
				}
			}
		} else if existing, ok := st.Resources[resource.RelativePath]; ok && existing.URL != "" && existing.URL != resource.URL {
			reason = "url-changed"
		}

		if reason == "" {
			skipped++
			continue
		}
		items = append(items, model.DownloadItem{
			Resource: resource,
			Reason:   reason,
		})
	}
	return items, skipped, st, nil
}

func verifyResources(outputDir string, resources []model.Resource) ([]string, []string, error) {
	missing := make([]string, 0)
	mismatch := make([]string, 0)
	for _, resource := range resources {
		localPath := filepath.Join(outputDir, filepath.FromSlash(resource.RelativePath))
		if _, err := os.Stat(localPath); err != nil {
			missing = append(missing, resource.RelativePath)
			continue
		}
		if resource.Hash == "" {
			continue
		}
		match, err := integrity.FileMatchesManifestHash(localPath, resource.Hash)
		if err != nil {
			return nil, nil, err
		}
		if !match {
			mismatch = append(mismatch, resource.RelativePath)
		}
	}
	return missing, mismatch, nil
}

func sampleItems(items []model.DownloadItem, limit int) []model.DownloadItem {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func printSummary(command string, summary model.Summary, pr *printer) {
	pr.Title(strings.Title(command) + " Summary")
	pr.KV("Seed files fetched", summary.SeedsFetched)
	pr.KV("Filecollection manifests parsed", summary.ManifestCount)
	pr.KV("Manifest-derived resources", summary.ResourceCount)
	pr.KV("Planned downloads", summary.PlannedDownloads)
	pr.KV("Skipped existing", summary.SkippedResources)
}

func printGroupedPaths(paths []string, title string, pr *printer) {
	grouped := map[string]int{}
	for _, rel := range paths {
		grouped[groupKey(rel)]++
	}

	type pair struct {
		Key   string
		Count int
	}
	pairs := make([]pair, 0, len(grouped))
	for key, count := range grouped {
		pairs = append(pairs, pair{Key: key, Count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Count == pairs[j].Count {
			return pairs[i].Key < pairs[j].Key
		}
		return pairs[i].Count > pairs[j].Count
	})

	limit := 15
	if len(pairs) < limit {
		limit = len(pairs)
	}
	if limit == 0 {
		return
	}
	pr.Section(title)
	for _, item := range pairs[:limit] {
		pr.Printf("%4d  %s\n", item.Count, item.Key)
	}
}

func printCategoryBreakdownResources(resources []model.Resource, title string, pr *printer) {
	counts := map[string]int{}
	for _, resource := range resources {
		counts[resource.Category]++
	}
	printCounts(title, counts, pr)
}

func printCategoryBreakdownPlan(plan []model.DownloadItem, title string, pr *printer) {
	counts := map[string]int{}
	for _, item := range plan {
		counts[item.Resource.Category]++
	}
	printCounts(title, counts, pr)
}

func printReasonBreakdown(plan []model.DownloadItem, pr *printer) {
	counts := map[string]int{}
	for _, item := range plan {
		counts[item.Reason]++
	}
	printCounts("Planned downloads by reason", counts, pr)
}

func printCounts(title string, counts map[string]int, pr *printer) {
	if len(counts) == 0 {
		return
	}
	type pair struct {
		Key   string
		Count int
	}
	pairs := make([]pair, 0, len(counts))
	for key, count := range counts {
		pairs = append(pairs, pair{Key: key, Count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Count == pairs[j].Count {
			return pairs[i].Key < pairs[j].Key
		}
		return pairs[i].Count > pairs[j].Count
	})
	pr.Section(title)
	for _, item := range pairs {
		pr.Printf("%6d  %s\n", item.Count, item.Key)
	}
}

func groupKey(rel string) string {
	parts := strings.Split(rel, "/")
	switch {
	case len(parts) >= 2 && parts[0] == "spacemap" && parts[1] == "3d":
		if len(parts) >= 3 {
			return strings.Join(parts[:3], "/")
		}
	case len(parts) >= 3 && parts[0] == "spacemap" && parts[1] == "graphics":
		return strings.Join(parts[:3], "/")
	case len(parts) >= 2:
		return strings.Join(parts[:2], "/")
	}
	return rel
}

func printSample(title string, items []string, limit int, pr *printer) {
	if len(items) == 0 {
		return
	}
	pr.Section(title)
	if len(items) < limit {
		limit = len(items)
	}
	for _, item := range items[:limit] {
		pr.Printf("  %s\n", item)
	}
	if len(items) > limit {
		pr.Printf("  ... and %d more\n", len(items)-limit)
	}
}

func parseCategories(csv string) map[string]bool {
	result := map[string]bool{}
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result[part] = true
	}
	if len(result) == 0 {
		result["all"] = true
	}
	return result
}

func parseLanguages(csv string) map[string]bool {
	result := map[string]bool{}
	for _, part := range strings.Split(csv, ",") {
		part = discovery.CanonicalLanguageCode(part)
		if part == "" {
			continue
		}
		result[part] = true
	}
	if len(result) == 0 {
		result["en"] = true
	}
	return result
}

func resolveLanguages(ctx context.Context, cfg config, seeds []model.Seed, pr *printer) (map[string]bool, error) {
	if len(cfg.Languages) == 0 {
		return map[string]bool{"en": true}, nil
	}
	if !cfg.Languages["all"] {
		return cfg.Languages, nil
	}

	live, err := discovery.DiscoverLiveLanguages(ctx, cfg.BaseURL)
	if err == nil && len(live) > 0 {
		resolved := map[string]bool{}
		for _, lang := range live {
			resolved[lang] = true
		}
		pr.Status("DISCOVER", fmt.Sprintf("live languages: %s", strings.Join(live, ", ")))
		return resolved, nil
	}

	fallback := discovery.ResolveBootstrapLanguages(cfg.OutputDir, seeds, cfg.Languages)
	resolved := map[string]bool{}
	for _, lang := range fallback {
		resolved[lang] = true
	}
	if len(fallback) > 0 {
		pr.Status("DISCOVER", fmt.Sprintf("fallback languages: %s", strings.Join(fallback, ", ")))
		return resolved, nil
	}
	return resolved, err
}

func prepareSeeds(ctx context.Context, cfg config, pr *printer, probeOptional bool) (config, []model.Seed, error) {
	seeds, err := discovery.DiscoverSeeds(cfg.OutputDir)
	if err != nil {
		return cfg, nil, err
	}

	resolvedLanguages, err := resolveLanguages(ctx, cfg, seeds, pr)
	if err != nil {
		return cfg, nil, err
	}
	cfg.Languages = resolvedLanguages
	seeds = discovery.AddLanguageBootstrapSeeds(cfg.OutputDir, seeds, cfg.Languages)

	if !probeOptional {
		return cfg, seeds, nil
	}

	probedSeeds, skipped, err := discovery.DiscoverAvailableSeeds(ctx, cfg.BaseURL, seeds)
	if err != nil {
		return cfg, nil, err
	}
	if len(skipped) > 0 {
		pr.Status("DISCOVER", fmt.Sprintf("skipping %d optional seeds that returned 404", len(skipped)))
	}
	return cfg, probedSeeds, nil
}

func matchesCategory(category string, allowed map[string]bool) bool {
	if len(allowed) == 0 || allowed["all"] {
		return true
	}
	return allowed[category]
}

func joinSeedURL(baseURL, relativePath string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(filepath.ToSlash(relativePath), "/")
}

func countFetchedSeeds(seeds []model.Seed, allowed map[string]bool, languages map[string]bool, includeCore bool) int {
	count := 0
	for _, seed := range seeds {
		if seed.Category == "core" && !includeCore {
			if strings.HasSuffix(seed.RelativePath, ".swf") || seed.RelativePath == "crossdomain.xml" {
				continue
			}
		}
		if matchesCategory(seed.Category, allowed) && discovery.MatchesLanguagePath(seed.RelativePath, languages) {
			count++
		}
	}
	return count
}

func printUsage(pr *printer) {
	pr.Title("Usage: go run ./cmd [command] [flags]")
	pr.Println()
	pr.Println("Commands:")
	pr.Println("  sync             Fetch latest manifests/core files and download missing resources")
	pr.Println("  plan             Fetch latest manifests and print what would be downloaded")
	pr.Println("  fetch-manifests  Refresh local manifest/template files only")
	pr.Println("  verify           Compare local files against the current local manifests")
	pr.Println()
	pr.Println("Common flags:")
	pr.Println("  --base-url       Default: https://www.darkorbit.com")
	pr.Println("  --output         Default: darkorbit-files")
	pr.Println("  --concurrency    Default: 8")
	pr.Println("  --min-concurrency Default: 1")
	pr.Println("  --auto-tune-concurrency Default: true")
	pr.Println("  --request-interval Default: 250ms")
	pr.Println("  --category       Default: all")
	pr.Println("  --languages      Default: en")
	pr.Println("  --force          Redownload resources even if they already exist")
	pr.Println("  --log-file       Default: app.log")
}

func newDownloader(cfg config) *downloader.Client {
	return downloader.NewWithConfig(downloader.Config{
		Timeout:             90 * time.Second,
		RequestInterval:     cfg.RequestInterval,
		MaxCooldown:         20 * time.Second,
		MaxConcurrency:      cfg.Concurrency,
		MinConcurrency:      cfg.MinConcurrency,
		AutoTuneConcurrency: cfg.AutoTuneConcurrency,
	})
}
