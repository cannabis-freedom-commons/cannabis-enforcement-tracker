// Loads the shared index of all reports (/api/index.json) once per page, for the map and the home
// page's filters. Rows have short keys: see indexRow in src/lib/reports.js.
export type Row = { i: string; d: string; at?: string; c: string; t: string; a: string[]; ti: string; s: string; p: string | null; ll: [number, number] | null; pr: string | null; f: [string, string][]; k: string; g?: string[]; im?: string;
  n?: Record<string, number | string>; st?: string };
export type Index = { v: number; countries: Record<string, string>; reports: Row[] };

let pending: Promise<Index> | null = null;
export function loadIndex(base: string): Promise<Index> {
  pending ??= fetch(`${base}api/index.json`).then((r) => r.json());
  return pending;
}

// a report about this country, or about one of its states ("us" matches "us/ca")
export const inCountry = (row: Row, id: string) => row.c === id || row.c.startsWith(`${id}/`);
