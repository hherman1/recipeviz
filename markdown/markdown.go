// Package markdown renders Markdown containing recipe dependency diagrams.
package markdown

import (
	"bytes"
	"fmt"

	"github.com/hherman1/recipeviz/recipe"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

var (
	converter        = newConverter(false)
	trustedConverter = newConverter(true)
)

// newConverter builds a Markdown converter that decorates recipe blocks with
// diagrams. Raw HTML in the source is stripped unless allowRawHTML is set.
func newConverter(allowRawHTML bool) goldmark.Markdown {
	options := []renderer.Option{renderer.WithNodeRenderers(util.Prioritized(
		&recipeBlockRenderer{Config: html.NewConfig()},
		100,
	))}
	if allowRawHTML {
		options = append(options, html.WithUnsafe())
	}
	return goldmark.New(goldmark.WithRendererOptions(options...))
}

// recipeBlockRenderer preserves normal fenced code output while decorating
// recipe blocks with diagrams.
type recipeBlockRenderer struct {
	html.Config
}

func (r *recipeBlockRenderer) RegisterFuncs(registerer renderer.NodeRendererFuncRegisterer) {
	registerer.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
}

func (r *recipeBlockRenderer) renderFencedCodeBlock(
	out util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		_, _ = out.WriteString("</code></pre>\n")
		return ast.WalkContinue, nil
	}

	block := node.(*ast.FencedCodeBlock)
	language := block.Language(source)
	if bytes.Equal(language, []byte("recipe")) {
		sourceLine := bytes.Count(source[:block.Info.Segment.Start], []byte{'\n'}) + 1
		var recipeSource bytes.Buffer
		for i := range block.Lines().Len() {
			line := block.Lines().At(i)
			_, _ = recipeSource.Write(line.Value(source))
		}
		parsed, err := recipe.Parse(recipeSource.String())
		if err != nil {
			return ast.WalkStop, fmt.Errorf("recipe block on line %d: parsing recipe: %v", sourceLine, err)
		}
		forest, err := recipe.Transform(parsed)
		if err != nil {
			return ast.WalkStop, fmt.Errorf("recipe block on line %d: transforming recipe: %v", sourceLine, err)
		}
		_, _ = out.WriteString(recipe.Render(forest))
		_ = out.WriteByte('\n')
	}

	_, _ = out.WriteString("<pre><code")
	if language != nil {
		_, _ = out.WriteString(` class="language-`)
		r.Writer.Write(out, language)
		_ = out.WriteByte('"')
	}
	_ = out.WriteByte('>')
	for i := range block.Lines().Len() {
		line := block.Lines().At(i)
		r.Writer.RawWrite(out, line.Value(source))
	}
	return ast.WalkContinue, nil
}

// Render converts CommonMark to an HTML fragment and inserts a dependency SVG
// immediately before each fenced recipe code block. Raw HTML in the source is
// stripped. An invalid recipe aborts conversion without returning partial HTML.
func Render(source []byte) (string, error) {
	return render(converter, source)
}

// RenderTrusted is Render with raw HTML in the source copied through to the
// output verbatim, which allows embedded media such as <video> and <iframe>.
// Use it only for Markdown you control.
func RenderTrusted(source []byte) (string, error) {
	return render(trustedConverter, source)
}

func render(converter goldmark.Markdown, source []byte) (string, error) {
	var out bytes.Buffer
	if err := converter.Convert(source, &out); err != nil {
		return "", fmt.Errorf("converting Markdown to HTML: %v", err)
	}
	return out.String(), nil
}
