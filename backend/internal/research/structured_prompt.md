
## Structured data (this section is part of the JSON contract)

Besides `figures`, a drafted result (new_event or existing_case) may give `details`, and must give
`data_quotes` for every figure and detail it states. Only what a source states for this event;
omit every key the sources do not give. An omitted key means "not reported"; give 0 only when a
source says none ("no one was arrested").

Extra figures (all optional, same rules as the others):
- plants_destroyed and kg_seized stay the totals (seized or destroyed). Split them only when a
  source does: plants_seized (taken away, not destroyed), kg_destroyed (burned or destroyed).
- convicted, acquitted, sentenced, released (freed without charge or on bail), injured;
- sites (plantations, houses or other places raided or destroyed), seeds (a count), vehicles
  (confiscated), cash (money confiscated: amount and currency).

`details` (all keys optional; ids from the lists below):
- agencies: the bodies that acted, as registry ids where one fits: {AGENCIES}; otherwise the
  agency's English name as published ("Kano State Police Command").
- agency_types: {agency_type}
- products: [{"product": id, "kg": n, "plants": n}] per cannabis product, with the quantity a
  source gives for it. Products: {product}
- activity (what the people did or were accused of): {activity}
- setting (where): {setting}
- environment (how the cannabis was grown): {environment}
- charges: the charges as stated, short ("cultivating cannabis without a licence").
- case_stage (how far the case has gone): {case_stage}
- sentence_types: {sentence_type}
- force: {force}
- period: {"start": "YYYY-MM-DD", "end": "YYYY-MM-DD"} when the operation ran over several days.
- date_precision: {date_precision}
- region: the state, province or district, in English.
- route: {"from": [country ids], "to": [country ids], "crossing": "named border post"} for
  smuggled cannabis.
- part_of: the slug of an existing report on the campaign this event is one component of.
- related: slugs of existing reports on other stages or parts of the same case.

`data_quotes`: [{"field": "figures.arrested", "source": 1, "quote": "exact words"}] — one or more
for every figure and every details key except part_of, related, date_precision and region. The
quote is copied verbatim from the cited source, like a fact's quote, and shows the value: the
count, the agency, the product, the place. One quote may support several fields (give it once per
field). The service checks each quote: a value with no quote, or whose quote is not in its opened
source, is left out with an open question. The tracker_check_result tool reports both.

Never record a person's name, address or other identifiers in details; counts and outcomes only.
