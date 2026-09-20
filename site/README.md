# Gyst website

One-page static Astro site for explaining what Gyst is, why an engineering team
might use it, and how the current developer preview works.

## Development

```sh
npm install
npm run dev
```

Build the static site with:

```sh
npm run build
```

The generated site is written to `dist/`. The page has no external runtime
assets, analytics, account requirement, or network dependency.

## Communication assets

- [`icon-search-map.md`](icon-search-map.md) records the visual nouns considered,
  selected Noun Project icons, and the job each icon performs.
- [`public/icons/README.md`](public/icons/README.md) contains human-readable
  attribution for the locally bundled CC BY 3.0 icon assets.
- [`public/icons/icon-licenses.json`](public/icons/icon-licenses.json) is the
  machine-readable attribution record.

The development-journey example is illustrative. Its people, revisions, costs,
and seven-day schedule explain the model; they are not customer results.
