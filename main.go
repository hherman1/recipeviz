package main

import (
	"fmt"
	"os"

	"github.com/hherman1/recipeviz/recipe"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: recipeviz RECIPE")
		os.Exit(2)
	}
	source, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "recipeviz: reading recipe: %v\n", err)
		os.Exit(1)
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
