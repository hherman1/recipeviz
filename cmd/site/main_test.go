package main

import (
	"compress/gzip"
	"encoding/base64"
	"io"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

// samplePage exercises every feature a recipe page may use.
const samplePage = `---
title: Butter Test
description: A short blurb.
tags: quick, one-pot
source: https://www.example.com/butter
---

` + "```recipe" + `
BUTTER: 4 oz butter
melt BUTTER
` + "```" + `

Some notes with an image.

![The dish](media/dish.jpg)

<video controls src="media/dish.mp4"></video>

` + "```recipe" + `
SALT: a pinch of salt
add SALT
` + "```" + `
`

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"butter-test.md":     {Data: []byte(samplePage)},
		"_template.md":       {Data: []byte("---\ntitle: Skipped\n---\n\nnot a page\n")},
		"media/dish.jpg":     {Data: []byte("jpeg bytes")},
		"media/.gitkeep":     {Data: []byte("")},
		"media/nested/a.png": {Data: []byte("png bytes")},
	}
}

func TestBuildRendersPagesBrowseAndMedia(t *testing.T) {
	files, err := build(testFS())
	if err != nil {
		t.Fatalf("build site: %v", err)
	}
	for _, name := range []string{"browse.html", "site.css", "recipes/butter-test.html", "recipes/media/dish.jpg", "recipes/media/nested/a.png"} {
		if _, ok := files[name]; !ok {
			t.Errorf("generated site is missing %v", name)
		}
	}
	if _, ok := files["recipes/media/.gitkeep"]; ok {
		t.Error("dotfiles in media were published")
	}
	if _, ok := files["recipes/_template.md.html"]; ok {
		t.Error("underscore prefixed file was published")
	}

	page := string(files["recipes/butter-test.html"])
	diagram := strings.Index(page, "<svg ")
	source := strings.Index(page, `<code class="language-recipe">`)
	body := strings.Index(page, `<div class="body">`)
	if diagram < 0 || source < 0 || body < 0 {
		t.Fatalf("page is missing its card, source or body:\n%s", page)
	}
	if !(diagram < source && source < body) {
		t.Errorf("card is not at the top: diagram at %d, source at %d, body at %d", diagram, source, body)
	}
	for _, want := range []string{
		"<title>Butter Test · recipeviz</title>",
		`<p class="lede-text">A short blurb.</p>`,
		"<li>quick</li>",
		`style="aspect-ratio: `,
		`<img src="media/dish.jpg" alt="The dish">`,
		`<video controls src="media/dish.mp4"></video>`,
		`<a href="https://www.example.com/butter" rel="noreferrer">Recipe from example.com</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	// The second recipe block keeps the inline diagram markdown.Render gives it.
	if got := strings.Count(page[body:], "<svg "); got != 1 {
		t.Errorf("body contains %d diagrams, want 1", got)
	}

	browse := string(files["browse.html"])
	for _, want := range []string{`href="recipes/butter-test.html"`, "<h2>Butter Test</h2>", "<p>A short blurb.</p>", "<svg "} {
		if !strings.Contains(browse, want) {
			t.Errorf("browse page does not contain %q", want)
		}
	}
}

func TestBuildLinksToThePlayground(t *testing.T) {
	files, err := build(testFS())
	if err != nil {
		t.Fatalf("build site: %v", err)
	}
	page := string(files["recipes/butter-test.html"])
	_, rest, found := strings.Cut(page, `href="../index.html?recipe=`)
	if !found {
		t.Fatalf("page has no playground link:\n%s", page)
	}
	encoded, _, _ := strings.Cut(rest, `"`)
	compressed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode playground link: %v", err)
	}
	reader, err := gzip.NewReader(strings.NewReader(string(compressed)))
	if err != nil {
		t.Fatalf("decompress playground link: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read playground link: %v", err)
	}
	if want := "BUTTER: 4 oz butter\nmelt BUTTER"; string(decoded) != want {
		t.Errorf("playground link decodes to %q, want %q", decoded, want)
	}
}

func TestBuildRejectsBrokenPages(t *testing.T) {
	for name, test := range map[string]struct{ source, want string }{
		"no front matter": {
			source: "# Title\n\n```recipe\nA: salt\nuse A\n```\n",
			want:   "front matter",
		},
		"unterminated front matter": {
			source: "---\ntitle: Broken\n",
			want:   "never closed",
		},
		"unknown key": {
			source: "---\ntitle: Broken\nauthor: me\n---\n",
			want:   `unknown front matter key "author"`,
		},
		"no title": {
			source: "---\ndescription: Broken\n---\n\n```recipe\nA: salt\nuse A\n```\n",
			want:   "missing a title",
		},
		"no recipe block": {
			source: "---\ntitle: Broken\n---\n\nJust prose.\n",
			want:   "no ```recipe block",
		},
		"source is not a URL": {
			source: "---\ntitle: Broken\nsource: cabbages.example\n---\n\n```recipe\nA: salt\nuse A\n```\n",
			want:   "not an http or https URL",
		},
		"invalid recipe": {
			source: "---\ntitle: Broken\n---\n\n```recipe\nA: salt\nB: first A\nC: second A\n```\n",
			want:   "transforming recipe",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := build(fstest.MapFS{"broken.md": {Data: []byte(test.source)}})
			if err == nil {
				t.Fatal("build succeeded")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error %q does not contain %q", err, test.want)
			}
		})
	}
}

func TestSplitCardIgnoresRecipeBlocksInsideOtherFences(t *testing.T) {
	body := "````markdown\n```recipe\nnot the card\n```\n````\n\n```recipe\nA: salt\nuse A\n```\n"
	card, rest, err := splitCard(body)
	if err != nil {
		t.Fatalf("split card: %v", err)
	}
	if want := "A: salt\nuse A"; card != want {
		t.Errorf("card = %q, want %q", card, want)
	}
	if !strings.Contains(rest, "not the card") {
		t.Errorf("the nested block was removed from the body: %q", rest)
	}
	if strings.Contains(rest, "A: salt") {
		t.Errorf("the card was left in the body: %q", rest)
	}
}

// TestCheckedInSiteIsUpToDate guards the generated pages, which are committed
// because GitHub Pages serves docs/ directly with no build step.
func TestCheckedInSiteIsUpToDate(t *testing.T) {
	files, err := build(os.DirFS("../../recipes"))
	if err != nil {
		t.Fatalf("build site: %v", err)
	}
	if stale := checkStale("../../docs", files); len(stale) > 0 {
		t.Fatalf("docs/ is out of date, rerun `go run ./cmd/site`: %v", stale)
	}
}
