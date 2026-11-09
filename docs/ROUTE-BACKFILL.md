# Finding more smuggling routes

The `/routes` page (`src/pages/routes.astro`, `src/lib/routes.js`) draws every report whose
`details.route` names where the cannabis came from and where it was going. Many reports still
have no route although their story is a smuggling case. This guide is for an agent adding the
missing ones. Read `docs/WRITING.md` (section 5, details and `data_sources`) first.

## What a route is

```yaml
details:
  route: {from: [th], to: [in], crossing: Sonauli}
data_sources:
  details.route: [1, 2]
```

- `from` and `to` are country ids from `src/data/countries.json` (a state id such as `us/ca` or
  `ca/on` where the report's own country is a state). Both ends are required for the map.
- `crossing` is optional: the named border post, bridge, port or checkpoint (`Woodlands
  Checkpoint`, `Malaba`, `Qamar Bridge`). Not a country, and not "via X".
- Transit doesn't make a third end. Thailand to the UK via Hong Kong is `from: [th], to: [gb]`.
- Several destinations are allowed when the source names them (`to: [es, gb]`). Each pair becomes
  a line, so don't list countries the source only mentions in passing.
- `details.route` in `data_sources` lists only sources that state the route, using the numbers of
  the report's own `sources`. Indent both lines like the other keys in the same block. Some
  reports use four spaces.

## Which reports to look at

1. List reports with no `details.route` whose `activity` includes `trafficking` or `smuggling`, or
   whose `setting` includes `airport`, `border`, `port` or `mail`. Next take the seizures with
   `kg_seized` of 50 kg or more. Skip the reports listed under "Already reviewed" below.
2. For each one, read the report text first. If it names both ends, use the citation on that
   sentence.
3. If the text names only one end or none, **read the cited sources** (the `url`, or the
   `archived` copy when the page is gone). Sources often give the flight ("arrived from
   Bangkok"), the ship's port of departure, the truck's origin or the destination named by
   police. When a source states the route:
   - add the route and its source number;
   - add one plain sentence to the report text saying it, with the citation
     (`The suitcase had come from Bangkok. [@2]`), following `docs/WRITING.md`. A detail must
     not exist only in the frontmatter.

## When not to add a route

- Nationality only ("a Thai national", "Albanian gardeners"): not a route.
- Countries that only helped (a tip from the NCA, DEA support, a joint operation).
- General patterns ("Comoros is a transit hub", "most hashish in Egypt comes from Morocco").
  Those describe a market, not this case.
- Planned exports never shipped, seeds or equipment, and loads that are only cocaine, heroin,
  meth or synthetic cannabinoids.
- A load seized at sea with neither end stated.
- "Possibly" or "probably" origins, unless an authority states it as its finding. Then the
  report's text should say "police believed…".
- Martinique, Guadeloupe, French Guiana and Mayotte are stored under `fr` and would draw a line
  to mainland France. Leave them out until they have their own country ids. Réunion (`re`) is
  fine.
- A domestic route (`from` and `to` in the same country) adds nothing to the map. Add it only
  when the report is about moving cannabis between two states or provinces.
- Anything already in `details.route`. Don't change an existing route without a source that
  contradicts it.

## Already reviewed

Commit `afc7ffffb` reviewed 790 candidates found by searching report texts (not sources) for
another country named next to a smuggling word, and added 316 routes. The 474 reports it left
alone had no route in their text. Their sources weren't read, so they are the best next targets:
the reports changed by no route commit that name a second country.

## Checking and committing

- After each batch, check that every changed file still parses and has both keys. The quickest
  check is `npm test` followed by `npm run build`, which validates every report against
  `src/content.config.ts`.
- Look at `/routes/` in `dist/routes/index.html`: the tiles at the top give the reports, routes
  and countries.
- Others work here at the same time. Run `git status` first, commit only the report files you
  changed (`git add` them by name), and use a message like "Record smuggling routes stated in N
  more reports". Then `git pull --rebase && git push`.
- Don't start dev or preview servers.

## Report back

Give the number of reports checked, routes added, and how many came from reading sources rather
than the report text. List the reports where the sources disagree on origin or destination;
those stay without a route.
