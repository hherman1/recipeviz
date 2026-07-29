package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hherman1/recipeviz/markdown"
	"github.com/hherman1/recipeviz/recipe"
)

func main() {
	htmlOutput := flag.Bool("html", false, "render Markdown with diagrams above recipe code blocks")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: recipeviz [-html] FILE")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	source, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "recipeviz: reading input: %v\n", err)
		os.Exit(1)
	}

	if *htmlOutput {
		output, err := markdown.Render(source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "recipeviz: %v\n", err)
			os.Exit(1)
		}
		if _, err := fmt.Print(output); err != nil {
			fmt.Fprintf(os.Stderr, "recipeviz: writing HTML: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ast, err := recipe.Parse(string(source))
	if err != nil {
		fmt.Fprintf(os.Stderr, "recipeviz: parsing recipe: %v\n", err)
		os.Exit(1)
	}
	forest, err := recipe.Transform(ast)
	if err != nil {
		fmt.Fprintf(os.Stderr, "recipeviz: transforming recipe: %v\n", err)
		os.Exit(1)
	}
	if _, err := fmt.Print(recipe.Render(forest)); err != nil {
		fmt.Fprintf(os.Stderr, "recipeviz: writing SVG: %v\n", err)
		os.Exit(1)
	}
}
