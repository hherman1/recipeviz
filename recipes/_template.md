---
title: Recipe Name
description: One sentence for the browse card and the page subtitle.
tags: dinner, one-pot
---

```recipe
ONION: one onion
OIL: 1 tbsp oil
MIX: saute ONION OIL
serve MIX
```

Everything below the first `recipe` block is optional. It is ordinary Markdown,
so headings, lists, links, blockquotes and tables all work.

## Images

Put files in `recipes/media/` and reference them relative to this page:

![The finished dish](media/example.jpg)

## Video and embeds

Raw HTML is passed through, so embedded players work too:

<video controls src="media/example.mp4"></video>

## More diagrams

A second `recipe` block anywhere below the first renders its own diagram in
place, above its source — useful for a sauce or a component with its own map.

```recipe
BUTTER: 4 oz butter
FLOUR: 1/4 cup flour
ROUX: cook together until blond BUTTER FLOUR
```

---

Files whose name starts with `_`, like this one, are skipped by the generator.
Copy it to `recipes/<slug>.md` to start a new page; the slug becomes the URL.
Then run `go run ./cmd/site` to regenerate `docs/`.
