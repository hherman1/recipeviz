package markdown

import (
	"strings"
	"testing"
)

func TestRenderInsertsDiagramBeforeEachRecipeBlock(t *testing.T) {
	source := []byte(`# My recipe

Got this from [xyz.com](https://xyz.com).

` + "```recipe" + `
A: salt & <pepper>
serve A
` + "```" + `

I love it!

` + "```recipe" + `
B: second ingredient
use B
` + "```" + `
`)

	output, err := Render(source)
	if err != nil {
		t.Fatalf("render Markdown: %v", err)
	}
	for _, want := range []string{
		"<h1>My recipe</h1>",
		`<a href="https://xyz.com">xyz.com</a>`,
		"<p>I love it!</p>",
		`<pre><code class="language-recipe">`,
		"A: salt &amp; &lt;pepper&gt;",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("HTML does not contain %q:\n%s", want, output)
		}
	}
	if got := strings.Count(output, "</svg>\n"+`<pre><code class="language-recipe">`); got != 2 {
		t.Fatalf("SVG followed by recipe block count = %d, want 2:\n%s", got, output)
	}
}

func TestRenderLeavesOtherCodeBlocksAlone(t *testing.T) {
	output, err := Render([]byte("```go\nfmt.Println(\"hello\")\n```\n"))
	if err != nil {
		t.Fatalf("render Markdown: %v", err)
	}
	if strings.Contains(output, "<svg ") {
		t.Fatalf("non-recipe code block produced an SVG:\n%s", output)
	}
	if !strings.Contains(output, `<pre><code class="language-go">fmt.Println(&quot;hello&quot;)`) {
		t.Fatalf("Go code block was not rendered normally:\n%s", output)
	}
}

func TestRenderReportsInvalidRecipeBlockLine(t *testing.T) {
	_, err := Render([]byte("# My recipe\n\n```recipe\nA: ingredient\nB: first A\nC: second A\n```\n"))
	if err == nil {
		t.Fatal("render invalid recipe succeeded")
	}
	for _, want := range []string{
		"recipe block on line 3",
		"transforming recipe",
		"already incorporated",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestRenderReportsRecipeParseErrors(t *testing.T) {
	output, err := Render([]byte("# My recipe\n\n```recipe\n```\n"))
	if err == nil {
		t.Fatal("render empty recipe succeeded")
	}
	if output != "" {
		t.Fatalf("failed conversion returned partial HTML:\n%s", output)
	}
	if want := "recipe block on line 3: parsing recipe: recipe has no steps"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want text %q", err, want)
	}
}
