# recipeviz

`recipeviz` turns a plain-text recipe into an SVG dependency table.

## CLI

```sh
go run . sample.recipe > sample.svg
```

## Go package

The parser, forest transform, data structures, and renderer are available from
`github.com/hherman1/recipeviz/recipe`:

```go
ast, err := recipe.Parse(source)
if err != nil {
	return err
}
forest, err := recipe.Transform(ast)
if err != nil {
	return err
}
svg := recipe.Render(forest)
```

## Browser playground

`interactive/` contains a self-contained editor backed by the compiled TinyGo
WebAssembly module. Serve it over HTTP:

```sh
python3 -m http.server --directory interactive 8000
```

Then open http://localhost:8000. To rebuild the module with TinyGo 0.41:

```sh
GOTOOLCHAIN=go1.23.0 tinygo build -o interactive/recipeviz.wasm \
  -target wasm -no-debug ./cmd/wasm
cp "$(tinygo env TINYGOROOT)/targets/wasm_exec.js" interactive/wasm_exec.js
```

## Recipe syntax

Each nonblank line is a step. Declare a result with an alphanumeric name,
followed by `: `:

```text
BUTTER: 4 oz butter
melt BUTTER
SUGAR: 1 cup sugar
BATTER: mix BUTTER SUGAR
bake BATTER
```

Declared names at the end of a step are its inputs. An unnamed step with one
input carries that name forward, so `melt BUTTER` makes later uses of `BUTTER`
refer to the melted result. Results cannot branch: each result may be
incorporated by only one other step, and cycles are rejected.

Inspired by https://x.com/juanbuis/status/2082162851553398821.