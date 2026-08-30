// Command site renders the recipes directory into the static pages that
// GitHub Pages serves from docs: one page per recipe, with its dependency
// diagram at the top and any extra prose or media below, plus the browse
// index that links them all together.
//
// Usage:
//
//	go run ./cmd/site           # regenerate the pages under docs/
//	go run ./cmd/site -check    # report, without writing, whether docs/ is stale
package main

import (
	"bytes"
	"compress/gzip"
	"embed"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/hherman1/recipeviz/markdown"
	"github.com/hherman1/recipeviz/recipe"
)

//go:embed templates/*.html assets/*
var sources embed.FS

var templates = template.Must(template.ParseFS(sources, "templates/*.html"))

// frontMatterKeys are the fields a recipe page may declare. Anything else is a
// typo the author would rather hear about than silently lose.
var frontMatterKeys = []string{"title", "description", "tags"}

// viewBoxPattern captures the width and height of a rendered diagram.
var viewBoxPattern = regexp.MustCompile(`viewBox="0 0 (\d+) (\d+)"`)

// page is one recipe rendered into everything the templates need.
type page struct {
	Slug        string
	Title       string
	Description string
	Tags        []string

	// Recipe is the source of the hero recipe block, shown alongside the card.
	Recipe string
	// Diagram is the card itself: the recipe rendered as an inline SVG.
	Diagram template.HTML
	// Width and Height come from the diagram's viewBox, so the card can
	// reserve the right amount of space before the SVG is laid out.
	Width, Height string
	// PlaygroundQuery is a share link that opens the recipe in the playground.
	PlaygroundQuery string
	// Body is the Markdown following the hero recipe block: notes, images,
	// videos, further recipe blocks, or nothing at all.
	Body template.HTML
}

func main() {
	recipesDir := flag.String("recipes", "recipes", "directory of recipe Markdown pages")
	outputDir := flag.String("out", "docs", "GitHub Pages directory to write")
	check := flag.Bool("check", false, "report whether the generated pages are up to date instead of writing them")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: site [-recipes DIR] [-out DIR] [-check]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}

	files, err := build(os.DirFS(*recipesDir))
	if err != nil {
		fmt.Fprintf(os.Stderr, "site: %v\n", err)
		os.Exit(1)
	}
	if *check {
		if stale := checkStale(*outputDir, files); len(stale) > 0 {
			fmt.Fprintf(os.Stderr, "site: %v is out of date; rerun `go run ./cmd/site`:\n", *outputDir)
			for _, name := range stale {
				fmt.Fprintf(os.Stderr, "\t%v\n", name)
			}
			os.Exit(1)
		}
		return
	}
	if err := write(*outputDir, files); err != nil {
		fmt.Fprintf(os.Stderr, "site: %v\n", err)
		os.Exit(1)
	}
}

// build renders every page in the recipes filesystem, returning the generated
// site as slash separated paths relative to the output directory.
func build(recipes fs.FS) (map[string][]byte, error) {
	names, err := fs.Glob(recipes, "*.md")
	if err != nil {
		return nil, fmt.Errorf("listing recipe pages: %v", err)
	}
	var pages []*page
	for _, name := range names {
		// A leading underscore marks a file as notes for the author, such as
		// the page template, rather than a recipe to publish.
		if strings.HasPrefix(name, "_") {
			continue
		}
		source, err := fs.ReadFile(recipes, name)
		if err != nil {
			return nil, fmt.Errorf("reading %v: %v", name, err)
		}
		parsed, err := parsePage(strings.TrimSuffix(name, ".md"), source)
		if err != nil {
			return nil, fmt.Errorf("%v: %v", name, err)
		}
		pages = append(pages, parsed)
	}
	if len(pages) == 0 {
		return nil, errors.New("no recipe pages found")
	}
	slices.SortFunc(pages, func(a, b *page) int { return strings.Compare(a.Title, b.Title) })

	files := make(map[string][]byte)
	stylesheet, err := sources.ReadFile("assets/site.css")
	if err != nil {
		return nil, fmt.Errorf("reading stylesheet: %v", err)
	}
	files["site.css"] = stylesheet
	for _, parsed := range pages {
		rendered, err := execute("recipe.html", parsed)
		if err != nil {
			return nil, fmt.Errorf("rendering %v: %v", parsed.Slug, err)
		}
		files[path.Join("recipes", parsed.Slug+".html")] = rendered
	}
	browse, err := execute("browse.html", pages)
	if err != nil {
		return nil, fmt.Errorf("rendering the browse page: %v", err)
	}
	files["browse.html"] = browse

	// Media referenced by the pages travels with them, so `![](media/x.jpg)`
	// resolves the same in the source Markdown and the generated page.
	media, err := collectMedia(recipes)
	if err != nil {
		return nil, err
	}
	for name, content := range media {
		files[path.Join("recipes", name)] = content
	}
	return files, nil
}

// parsePage turns one Markdown file into a renderable page. The first fenced
// recipe block becomes the card; everything else becomes the body.
func parsePage(slug string, source []byte) (*page, error) {
	meta, body, err := splitFrontMatter(string(source))
	if err != nil {
		return nil, err
	}
	if meta["title"] == "" {
		return nil, errors.New("front matter is missing a title")
	}
	recipeSource, body, err := splitCard(body)
	if err != nil {
		return nil, err
	}

	ast, err := recipe.Parse(recipeSource)
	if err != nil {
		return nil, fmt.Errorf("parsing recipe: %v", err)
	}
	forest, err := recipe.Transform(ast)
	if err != nil {
		return nil, fmt.Errorf("transforming recipe: %v", err)
	}
	diagram := recipe.Render(forest)
	viewBox := viewBoxPattern.FindStringSubmatch(diagram)
	if viewBox == nil {
		return nil, errors.New("rendered diagram has no viewBox")
	}
	playground, err := playgroundQuery(recipeSource)
	if err != nil {
		return nil, err
	}
	rendered, err := markdown.RenderTrusted([]byte(body))
	if err != nil {
		return nil, err
	}

	var tags []string
	for _, tag := range strings.Split(meta["tags"], ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return &page{
		Slug:            slug,
		Title:           meta["title"],
		Description:     meta["description"],
		Tags:            tags,
		Recipe:          recipeSource,
		Diagram:         template.HTML(diagram),
		Width:           viewBox[1],
		Height:          viewBox[2],
		PlaygroundQuery: playground,
		Body:            template.HTML(rendered),
	}, nil
}

// splitFrontMatter separates a leading `---` delimited block of `key: value`
// lines from the Markdown that follows it.
func splitFrontMatter(source string) (map[string]string, string, error) {
	lines := strings.Split(source, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, "", errors.New("file does not begin with a `---` front matter block")
	}
	meta := make(map[string]string)
	for i, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			return meta, strings.Join(lines[i+2:], "\n"), nil
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, "", fmt.Errorf("front matter line %d is not `key: value`: %q", i+2, line)
		}
		if !slices.Contains(frontMatterKeys, key) {
			return nil, "", fmt.Errorf("unknown front matter key %q, want one of %v", key, frontMatterKeys)
		}
		if _, duplicate := meta[key]; duplicate {
			return nil, "", fmt.Errorf("duplicate front matter key %q", key)
		}
		meta[key] = strings.TrimSpace(value)
	}
	return nil, "", errors.New("front matter block is never closed by `---`")
}

// splitCard removes the first top level fenced recipe block from the body and
// returns its source, which the page renders as the card at the top.
func splitCard(body string) (string, string, error) {
	lines := strings.Split(body, "\n")
	open, start := "", -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if open != "" {
			// A fence closes on a run of its own character at least as long as
			// the run that opened it, with nothing but space after it.
			if run := runLength(trimmed, open[0]); run >= len(open) && strings.TrimSpace(trimmed[run:]) == "" {
				if start >= 0 {
					card := strings.Join(lines[start+1:i], "\n")
					rest := strings.Join(slices.Delete(slices.Clone(lines), start, i+1), "\n")
					return card, rest, nil
				}
				open = ""
			}
			continue
		}
		for _, marker := range []byte{'`', '~'} {
			run := runLength(trimmed, marker)
			if run < 3 {
				continue
			}
			open = trimmed[:run]
			if strings.TrimSpace(trimmed[run:]) == "recipe" {
				start = i
			}
			break
		}
	}
	if start >= 0 {
		return "", "", errors.New("the recipe block is never closed")
	}
	return "", "", errors.New("page has no ```recipe block to render as its card")
}

// runLength counts the run of one character at the start of a line.
func runLength(line string, character byte) int {
	length := 0
	for length < len(line) && line[length] == character {
		length++
	}
	return length
}

// playgroundQuery encodes a recipe the way the playground's share links do:
// gzipped, then URL safe base64 without padding.
func playgroundQuery(source string) (string, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(source)); err != nil {
		return "", fmt.Errorf("compressing the playground link: %v", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("compressing the playground link: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(compressed.Bytes()), nil
}

// collectMedia reads the optional media directory shared by the pages.
func collectMedia(recipes fs.FS) (map[string][]byte, error) {
	if _, err := fs.Stat(recipes, "media"); err != nil {
		return nil, nil
	}
	media := make(map[string][]byte)
	err := fs.WalkDir(recipes, "media", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Placeholders such as .gitkeep keep the directory in git without
		// belonging in the published site.
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		content, err := fs.ReadFile(recipes, name)
		if err != nil {
			return err
		}
		media[name] = content
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collecting media: %v", err)
	}
	return media, nil
}

func execute(name string, data any) ([]byte, error) {
	var out bytes.Buffer
	if err := templates.ExecuteTemplate(&out, name, data); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// write replaces the generated files in the output directory, removing pages
// left behind by recipes that have since been renamed or deleted.
func write(outputDir string, files map[string][]byte) error {
	previous, err := filepath.Glob(filepath.Join(outputDir, "recipes", "*.html"))
	if err != nil {
		return fmt.Errorf("listing existing pages: %v", err)
	}
	for _, name := range previous {
		if _, kept := files[path.Join("recipes", filepath.Base(name))]; kept {
			continue
		}
		if err := os.Remove(name); err != nil {
			return fmt.Errorf("removing stale page: %v", err)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		destination := filepath.Join(outputDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("creating %v: %v", filepath.Dir(destination), err)
		}
		if err := os.WriteFile(destination, files[name], 0o644); err != nil {
			return fmt.Errorf("writing %v: %v", destination, err)
		}
	}
	return nil
}

// checkStale returns the generated files that are missing from or differ on
// disk, in sorted order.
func checkStale(outputDir string, files map[string][]byte) []string {
	var stale []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		onDisk, err := os.ReadFile(filepath.Join(outputDir, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(onDisk, files[name]) {
			stale = append(stale, name)
		}
	}
	return stale
}
