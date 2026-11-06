// The CFI score in a year, from a trend of [year, score] points where it changed (src/data/countries.json).
export function scoreIn(trend, year) {
  if (!trend?.length) return null;
  let score = null;
  for (const [y, s] of trend) if (y <= year) score = s;
  return score;
}
