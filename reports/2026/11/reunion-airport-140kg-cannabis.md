---
added_at_source: git
added_at: "2026-10-29T13:10:07.000Z"
code: 60RR5V
date: 2026-11-03
country: re
place: Roland Garros Airport, Sainte-Marie
# Geographic reference: https://nominatim.openstreetmap.org/search?q=Roland+Garros+Airport%2C+Reunion&format=jsonv2&limit=3
coordinates: [-20.890223, 55.517318]
precision: locality
type: sentence
also: [arrest, seizure]
title: Réunion Paris mule jailed 5 years over 37 kg resin in record week
summary: Customs officers at Roland-Garros airport arrested a 20-year-old man arriving from Paris on 10 September 2026 with 37 kg of cannabis resin and 15 kg of cocaine. The court later found him guilty and sentenced him to five years in prison with continued detention and a five-year ban from Réunion.
figures:
    arrested: 2
    kg_seized: 194
    prison: 5 years
details:
  agency_types: [customs, police, prosecutor, court]
  case_stage: sentenced
  products: [{product: hashish, kg: 194}, {product: flower}]
  sentence_types: [prison]
  setting: [airport, prison]
data_sources:
  details.agency_types: [1, 8, 9, 5, 7, 3]
  details.case_stage: [1, 9, 6, 7]
  details.products: [5, 6, 7, 9, 3, 1]
  details.sentence_types: [1, 2, 9, 6, 7]
  details.setting: [1, 9, 4, 5, 6, 7]
  figures.prison: [1, 9, 6, 7, 2]
status: reported
created_by_model: opencode-go/deepseek-v4.1-flash#max
tags: [reunion, cannabis, customs, air-cargo, saint-denis]
sources:
  - title: 'La Réunion : plus de 200 kg de stupéfiants saisis en quelques jours'
    url: https://www.douane.gouv.fr/actualites/la-reunion-plus-de-200-kg-de-stupefiants-saisis-en-quelques-jours
    publisher: Direction générale des douanes et droits indirects
    date: 2026-09-15
    language: fr
  - title: '[Photos - Vidéo] À l''aéroport, les douaniers saisissent 140 kg d’herbe de cannabis dissimulés dans 40 cartons'
    url: https://imazpress.com/faits-divers/semaine-memorable-pour-les-douaniers-de-la-reunion-plus-de-200-kg-de-stupefiants-saisis-en-quelques-jours-dont-140-kg-de-cannabis-en-provenance-de-thailande
    publisher: Imaz Press Réunion
    date: 2026-09-21
    language: fr
  - title: 'Drogues à la Réunion : quand les saisies s''emballent'
    url: https://www.lequotidien.re/article/faits-divers/2026/09/17/drogues-a-la-reunion-quand-les-saisies-semballent
    publisher: Le Quotidien de La Réunion
    date: 2026-09-17
    language: fr
  - title: Une mule interceptée à l'aéroport Roland-Garros avec 17 kilos d'herbe de cannabis
    url: https://imazpress.com/faits-divers/une-mule-interceptee-a-l-aeroport-roland-garros-avec-17-kilos-d-herbe-de-cannabis
    publisher: Imaz Press Réunion
    date: 2026-09-10
    language: fr
  - title: 'Interpellée à l''aéroport : en provenance de Guyane, la mule voyageait en classe affaires avec 52 kilos de drogues'
    url: https://imazpress.com/toute-l-actu/saisie-record-effectuee-sur-une-mule-a-l-aeroport-de-roland-garros-15kg-de-coke-et-37-kg
    publisher: Imaz Press Réunion
    date: 2026-09-12
    language: fr
  - title: 'La Réunion : intercepté avec 52 kg de drogue dans ses valises, il est condamné à cinq ans de prison'
    url: https://www.lequotidien.re/article/faits-divers/2026/11/03/sur-le-top-10-vous-etes-numero-1
    publisher: Le Quotidien de La Réunion
    date: 2026-11-03
    language: fr
  - title: Cinq ans de prison ferme pour la mule de tous les records
    url: https://www.zinfos974.com/cinq-ans-de-prison-ferme-pour-la-mule-de-tous-les-records/
    publisher: Zinfos974
    date: 2026-10-29
    language: fr
  - title: 'Aéroport Roland-Garros : les douaniers saisissent 140 kg d’herbe de cannabis en provenance de Thaïlande'
    url: https://www.linfo.re/la-reunion/societe/la-reunion-140-kg-d-herbe-de-cannabis-saisis-dans-un-fret-aerien-en-provenance-de-thailande
    publisher: LINFO.re
    date: 2026-09-21
    language: fr
  - title: 'Lutte contre le narcotrafic : la Douane de La Réunion saisit près de 200 kg de stupéfiants en quelques jours'
    url: https://la1ere.franceinfo.fr/reunion/lutte-contre-le-narcotrafic-la-douane-de-la-reunion-saisit-pres-de-200-kg-de-stupefiants-en-quelques-jours-1740515.html
    publisher: Réunion La 1ère
    date: 2026-09-21
    language: fr
  - title: 'Aéroport Roland Garros : une mule arrêtée avec 17kg de cocaïne et 35 kg de résine de cannabis'
    url: https://www.linfo.re/la-reunion/faits-divers/aeroport-roland-garros-une-mule-arretee-avec-35-kg-de-resine-de-cannabis-et-17kg-de-cocaine
    publisher: LINFO.re
    date: 2026-09-11
    language: fr
open_questions:
  - The sources do not say whether the British woman or the Guianese man was charged, tried or sentenced.
  - The customs page is dated 15 September 2026, while Imaz Press dates the communiqué to 21 September 2026; the seizure dates of 9 and 10 September are consistent across sources.
  - The 194 kg cannabis figure is the sum of the 140 kg, 17 kg and 37 kg seizures; no single source states the total.
  - The article gives no separate date for the sentencing hearing; 3 November 2026 is its publication date and is used as the incident date.
  - The published value of between 2 and 3 million euros covers cocaine and cannabis together, so no cannabis-only value is recorded.
  - Cocaine quantities are mentioned only to explain the mixed seizure and are excluded from cannabis figures.
  - The article also describes a separate July interception in Bangkok with 47 kg of cannabis herb, a different event left out here.
  - Only one accessible source for the sentencing step could be opened; a second outlet's account could not be opened.
  - The new account dates the sentencing hearing to 28 October 2026, while the existing report uses the 3 November 2026 publication date as the incident date.
  - The reported 2,389,200-euro total covers cannabis resin and cocaine together, so no cannabis-only value can be recorded.
  - The page's only image is captioned as the Saint-Denis judicial court building and is not shown to depict the seizure, so no media entry is included.
  - LINFO.re gives the cannabis resin as 35 kg and the cocaine as 17 kg, while Imaz Press and the existing report give 37 kg and 15 kg; the existing figures are kept and the difference noted.
  - LINFO.re does not say whether this traveller was the man later sentenced to five years in October 2026; the matching date, Paris origin and record description suggest the same case, and the existing report records that sentence.
  - The submitted page's only image is captioned as an airport drug mule picture and is not established to show this seizure, so no media entry is included from it.
media:
  - kind: photo
    url: https://www.lequotidien.re/article/faits-divers/2026/11/03/sur-le-top-10-vous-etes-numero-1
    media_url: https://www.lequotidien.re/assets/images/cache/thumbnail_une_1/assets/images/articles/2026/11/03/6ae927a6e07936.13479071-336077.HR.jpg
    publisher: Le Quotidien de La Réunion
    caption: 'Customs photos of the September 2026 seizures including 15 kg of cocaine and 37 kg of cannabis resin carried by a Guyanese man arriving from Orly in business class. Photo: DR via Le Quotidien de La Réunion'
  - kind: photo
    url: https://la1ere.franceinfo.fr/reunion/lutte-contre-le-narcotrafic-la-douane-de-la-reunion-saisit-pres-de-200-kg-de-stupefiants-en-quelques-jours-1740515.html
    media_url: https://la1ere.franceinfo.fr/image/aw-WI6aXkuje7EWesk_KGEDwGeA/0x0:800x450/900x506/filters:format(webp):quality(80)/outremer%2F2026%2F09%2F21%2F6ab115fd72894990936219.jpg
    publisher: Réunion La 1ère
    caption: 'Cannabis herb seized by Réunion customs from Thai air freight bound for Amsterdam. Photo: Douane de La Réunion'
  - kind: photo
    url: https://la1ere.franceinfo.fr/reunion/lutte-contre-le-narcotrafic-la-douane-de-la-reunion-saisit-pres-de-200-kg-de-stupefiants-en-quelques-jours-1740515.html
    media_url: https://la1ere.franceinfo.fr/image/mv3tL1rhdshZ5S1yUMEx2d5bDSk/0x0:800x800/720x720/filters:format(webp):quality(80)/outremer%2F2026%2F09%2F21%2F6ab116236519c076597803.jpg
    publisher: Réunion La 1ère
    caption: 'The 140 kg of cannabis herb found in the air-freight consignment at Réunion. Photo: Douane de La Réunion'
  - kind: photo
    url: https://la1ere.franceinfo.fr/reunion/lutte-contre-le-narcotrafic-la-douane-de-la-reunion-saisit-pres-de-200-kg-de-stupefiants-en-quelques-jours-1740515.html
    media_url: https://la1ere.franceinfo.fr/image/Qz2KhaUJC9yciVBSADB3e_wrzNY/0x75:800x525/720x405/filters:quality(80)/outremer%2F2026%2F09%2F21%2F6ab116442ba09132722954.jpg
    publisher: Réunion La 1ère
    caption: 'Vacuum-sealed cannabis bags from the 140 kg cargo seizure. Photo: Douane de La Réunion'
---

Customs officers in Réunion seized 140 kg of cannabis herb from an air-cargo
consignment on the Bangkok–Amsterdam route on 10 September 2026. [@1,8,9] Agents at
the Saint-Denis Aéroport customs office selected the consignment, which was in
transit at Réunion. [@1,8] They targeted two pallets holding 40 cartons and
X-rayed them. [@1,8,9] Behind consumer goods they found vacuum-sealed plastic bags
hidden in black bubble wrap. [@1,8,9] In total they seized 258 bags of cannabis herb
weighing 140 kg. [@1,8,9] Customs called it Réunion's first seizure of Thai cannabis
herb from air freight in transit to Europe. [@1,8,9]

Less than 24 hours earlier, officers intercepted 17 kg of Thai cannabis herb on
a British woman who had flown via Chennai. [@1,9] She was placed in police custody
at the Saint-Denis territorial judicial police service. [@4] On 10 September a
young man from French Guiana arriving from Paris was arrested at Roland-Garros
airport with more than 50 kg of drugs. [@5,6,7,10] He was carrying 37 kg of cannabis
resin and 15 kg of cocaine. [@5,6,7,9] The authorities called the haul the largest
seizure ever made from a passenger in Réunion. [@5,7,10]

Le Quotidien put the Thai cannabis total at nearly 157 kg, the 140 kg cargo haul
plus the 17 kg courier seizure. [@3] The week's haul also included 37 kg of
cannabis resin from the Paris passenger. [@1] Customs said the operations
produced more than 200 kg of drugs in all. [@2,9] It said the seizures were
carried out under the prefectural « Zéro drogue » plan. [@1,9] It ended its
statement with the phrase « Pa ter là ! », which it translated as 'not here'.
[@1]

The prosecutor requested six years in prison, continued detention and a five-year ban from Réunion. [@6,7] The court found Justin M. guilty and sentenced him to five years in prison, with continued detention and a five-year ban from staying in Réunion. [@6,7]

The seized drugs were given a combined estimated total of 2,389,200 euros, with a 4,800-euro business-class ticket and a promised 8,000-euro payment. [@7] The hearing placed the sentencing on 28 October 2026. [@7] Thai customs officers intercepted him at departure for Réunion, confiscated the goods and let him continue, for an operation in which he said he was to receive 9,000 euros. [@7]

The prefect of Réunion, Patrice Latron, said about fifty mules had been arrested on the island over the previous year and that the trend had not weakened. [@10] He said travellers were now arriving from almost every destination served by the island's airlines, especially from Thailand, which he described as having liberalised cannabis use. [@10]
