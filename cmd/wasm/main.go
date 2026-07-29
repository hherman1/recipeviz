//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/hherman1/recipeviz/recipe"
)

// renderRecipe is the JavaScript boundary for the browser playground.
func renderRecipe(_ js.Value, args []js.Value) any {
	result := js.Global().Get("Object").New()
	result.Set("svg", "")
	result.Set("error", "")
	if len(args) != 1 {
		result.Set("error", "renderRecipe expects one recipe string")
		return result
	}

	ast, err := recipe.Parse(args[0].String())
	if err != nil {
		result.Set("error", err.Error())
		return result
	}
	forest, err := recipe.Transform(ast)
	if err != nil {
		result.Set("error", err.Error())
		return result
	}
	result.Set("svg", recipe.Render(forest))
	return result
}

func main() {
	render := js.FuncOf(renderRecipe)
	js.Global().Set("recipevizRender", render)
	select {}
}
