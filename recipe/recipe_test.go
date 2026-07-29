package recipe

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestSampleRecipe(t *testing.T) {
	source, err := os.ReadFile("../sample.recipe")
	if err != nil {
		t.Fatalf("read sample recipe: %v", err)
	}
	ast, err := Parse(string(source))
	if err != nil {
		t.Fatalf("parse sample recipe: %v", err)
	}
	if got, want := len(ast.Steps), 16; got != want {
		t.Fatalf("step count = %d, want %d", got, want)
	}
	if got, want := strings.Join(ast.Steps[7].Inputs, ","), "UBR,SG,V,COFFEE"; got != want {
		t.Fatalf("MIX1 inputs = %q, want %q", got, want)
	}

	forest, err := Transform(ast)
	if err != nil {
		t.Fatalf("transform sample recipe: %v", err)
	}
	if got, want := len(forest.Roots), 3; got != want {
		t.Fatalf("root count = %d, want %d", got, want)
	}
	bake := forest.Roots[2]
	if bake.Text != "bake 350* 30-40 minutes" || len(bake.Children) != 1 {
		t.Fatalf("bake root = %#v", bake)
	}
	fold := bake.Children[0]
	if fold.Text != "fold in" || len(fold.Children) != 5 {
		t.Fatalf("fold step = %#v", fold)
	}
	mix2 := fold.Children[0]
	if mix2.Text != "mix" || len(mix2.Children) != 2 {
		t.Fatalf("MIX2 step = %#v", mix2)
	}
	mix1 := mix2.Children[0]
	if mix1.Text != "mix" || len(mix1.Children) != 4 {
		t.Fatalf("MIX1 step = %#v", mix1)
	}
	if got, want := mix1.Children[0].Text, "melt"; got != want {
		t.Fatalf("first MIX1 input = %q, want %q", got, want)
	}

	svg := Render(forest)
	var width, height int
	if _, err := fmt.Sscanf(
		svg,
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`,
		&width,
		&height,
	); err != nil {
		t.Fatalf("read SVG dimensions: %v", err)
	}
	if width > 800 || height > 500 {
		t.Errorf("SVG dimensions = %dx%d, want a compact rendering", width, height)
	}
	if strings.Contains(svg, "\n") {
		t.Error("SVG contains unnecessary newlines")
	}
	if strings.Contains(svg, "dominant-baseline") {
		t.Error("SVG uses browser-dependent dominant baseline alignment")
	}
	if got, want := strings.Count(svg, "<line "), 6; got != want {
		t.Errorf("SVG line count = %d, want %d root separators and input closures", got, want)
	}
	coffeeStart := strings.Index(svg, ">1 shot espresso")
	if coffeeStart < 0 {
		t.Fatal("SVG does not contain the coffee step")
	}
	coffeeEnd := strings.Index(svg[coffeeStart:], "</text>")
	if coffeeEnd < 0 || !strings.Contains(svg[coffeeStart:coffeeStart+coffeeEnd], "</tspan><tspan") {
		t.Error("long coffee step was widened instead of wrapped")
	}

	decoder := xml.NewDecoder(strings.NewReader(svg))
	for {
		if _, err := decoder.Token(); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("rendered invalid XML: %v", err)
		}
	}
	for _, text := range []string{
		"Butter and flour an 8x8 in pan",
		"4oz (115 g) unsalted butter",
		"<tspan",
		"bake",
	} {
		if !strings.Contains(svg, text) {
			t.Errorf("SVG does not contain %q", text)
		}
	}
}

func TestTransformRejectsNonForests(t *testing.T) {
	tests := []struct {
		name   string
		source string
		error  string
	}{
		{
			name: "branch",
			source: `A: ingredient
B: first A
C: second A`,
			error: "already incorporated",
		},
		{
			name: "cycle",
			source: `A: from B
B: from A`,
			error: "dependency cycle",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ast, err := Parse(test.source)
			if err != nil {
				t.Fatalf("parse recipe: %v", err)
			}
			if _, err := Transform(ast); err == nil || !strings.Contains(err.Error(), test.error) {
				t.Fatalf("transform error = %v, want text %q", err, test.error)
			}
		})
	}
}

func TestParseRejectsDuplicateResults(t *testing.T) {
	_, err := Parse("A: first\nA: second")
	if err == nil || !strings.Contains(err.Error(), "already declared") {
		t.Fatalf("parse error = %v, want duplicate declaration error", err)
	}
}

func TestUnnamedStepCarriesInputForward(t *testing.T) {
	ast, err := Parse(`A: onions
B: oil
finely chop A
saute B A`)
	if err != nil {
		t.Fatalf("parse recipe: %v", err)
	}
	forest, err := Transform(ast)
	if err != nil {
		t.Fatalf("transform recipe: %v", err)
	}
	if got, want := len(forest.Roots), 1; got != want {
		t.Fatalf("root count = %d, want %d", got, want)
	}
	saute := forest.Roots[0]
	if got, want := saute.Children[0].Text, "finely chop"; got != want {
		t.Fatalf("first saute input = %q, want %q", got, want)
	}
	if got, want := saute.Children[1].Text, "oil"; got != want {
		t.Fatalf("second saute input = %q, want %q", got, want)
	}
}

func TestRenderEscapesText(t *testing.T) {
	ast, err := Parse("A: salt & <pepper>\nserve A")
	if err != nil {
		t.Fatalf("parse recipe: %v", err)
	}
	forest, err := Transform(ast)
	if err != nil {
		t.Fatalf("transform recipe: %v", err)
	}
	svg := Render(forest)
	if !strings.Contains(svg, "salt &amp; &lt;pepper&gt;") {
		t.Fatalf("SVG did not escape text:\n%s", svg)
	}
}
