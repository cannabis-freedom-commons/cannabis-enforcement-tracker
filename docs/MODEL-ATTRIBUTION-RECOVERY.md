# Historical model attribution recovery

Audit date: 2026-10-30. The audit began with 561 reports whose creator was recorded as
`unknown`. Only explicit, report-specific historical attribution was used. Existing known
creators were left unchanged.

## Result

Recovered 22 creators: 12 from the report's creation commit co-author and 10 from the
original CFI article's `model` field. The imported articles' creation and pre-transfer
model fields agree. The remaining 539 reports stay `unknown`:

- 133 tracker-native reports have no named model in their creation commit or original front matter.
- 31 imported articles have no recorded model in their original article metadata or creation commit.
- 375 other imports have no article-level attribution; their original CFI entry creation commits
  also do not identify a model.

Imports were traced through `cfi_id` into the CFI repository before its transfer commit
`a1cb631a90af6868bf2d5434ffd2a85ce3de3844`. The tracker import commit's Claude co-author
was **not** treated as the creator of the imported material. A generic Cursor co-author,
a neighbouring report's model, an unrelated edit, or the current model configuration is
not evidence of the original creator.

The audit also checked historical `created_by_model` changes. A Mauritius report briefly
carried a Muse Spark attribution during an accidental replacement; [the restoration
commit](https://github.com/cannabis-freedom-commons/cet/commit/a40af2d)
reinstated the original report. That discarded rewrite's model was not assigned to the
restored original.

Identifiers use vendor prefixes with the model labels preserved in history: `opus-5.5`,
`gpt-6` and `gpt-6.1-sol`. No API deployment alias or reasoning effort is inferred where
it was not recorded. In particular, the Claude co-author records the model name/version,
not a reasoning setting.

Only `created_by_model` changed in the reports. Their text, sources, dates, status and
other front matter were preserved. No update-model metadata was added.

## Evidence

| Report | Restored creator | Original evidence |
| --- | --- | --- |
| [seychelles-francois-resin-trafficking-2000.md](../reports/2000/07/seychelles-francois-resin-trafficking-2000.md) | `openai/gpt-6.1-sol` | [original CFI article model field: gpt-6.1-sol](https://github.com/burningtree/cannabis-freedom-index/blob/36d70ba612ae74b57c08de72c1050453c7f69dc0/articles/seychelles-francois-resin-trafficking-2000.md) |
| [malta-holmes-sentence-2011.md](../reports/2011/11/malta-holmes-sentence-2011.md) | `anthropic/opus-5.5` | [original CFI article model field: opus-5.5](https://github.com/burningtree/cannabis-freedom-index/blob/152245b1c135b8a2c68eecf579a86acb9355525f/articles/malta-holmes-sentence-2011.md) |
| [vanuatu-pest-barrier-cannabis-2017.md](../reports/2017/05/vanuatu-pest-barrier-cannabis-2017.md) | `openai/gpt-6` | [original CFI article model field: gpt-6](https://github.com/burningtree/cannabis-freedom-index/blob/8eb22ae0e9288511109383cfb6316521479008c7/articles/vanuatu-pest-barrier-cannabis-2017.md) |
| [serbia-cannabis-drying-sentence-2019.md](../reports/2019/01/serbia-cannabis-drying-sentence-2019.md) | `openai/gpt-6` | [original CFI article model field: gpt-6](https://github.com/burningtree/cannabis-freedom-index/blob/5c21608ed4cbb5e9191533927c83506833b1ccf5/articles/serbia-cannabis-drying-sentence-2019.md) |
| [togo-cannabis-bus-seizure-2020.md](../reports/2020/09/togo-cannabis-bus-seizure-2020.md) | `openai/gpt-6` | [original CFI article model field: gpt-6](https://github.com/burningtree/cannabis-freedom-index/blob/ad0709ceb4a9f927c3be2e236152827fcbdae95d/articles/togo-cannabis-bus-seizure-2020.md) |
| [tanzania-cultivation-sentence-upheld-2022.md](../reports/2022/03/tanzania-cultivation-sentence-upheld-2022.md) | `openai/gpt-6` | [original CFI article model field: gpt-6](https://github.com/burningtree/cannabis-freedom-index/blob/0b0166999421860d0b23b7397fa7cc982e616923/articles/tanzania-cultivation-sentence-upheld-2022.md) |
| [serbia-large-personal-stock-sentence-2024.md](../reports/2024/01/serbia-large-personal-stock-sentence-2024.md) | `openai/gpt-6` | [original CFI article model field: gpt-6](https://github.com/burningtree/cannabis-freedom-index/blob/5c21608ed4cbb5e9191533927c83506833b1ccf5/articles/serbia-large-personal-stock-sentence-2024.md) |
| [fiji-growers-sentences-reduced-2024.md](../reports/2024/05/fiji-growers-sentences-reduced-2024.md) | `openai/gpt-6` | [original CFI article model field: gpt-6](https://github.com/burningtree/cannabis-freedom-index/blob/ad0709ceb4a9f927c3be2e236152827fcbdae95d/articles/fiji-growers-sentences-reduced-2024.md) |
| [mozambique-cannabis-seller-sentence-reduced-2024.md](../reports/2024/05/mozambique-cannabis-seller-sentence-reduced-2024.md) | `openai/gpt-6` | [original CFI article model field: gpt-6](https://github.com/burningtree/cannabis-freedom-index/blob/23e18ffd98c1457156e5cb2d28d1c55cd89ccb23/articles/mozambique-cannabis-seller-sentence-reduced-2024.md) |
| [vanuatu-five-plants-remand-2024.md](../reports/2024/05/vanuatu-five-plants-remand-2024.md) | `openai/gpt-6` | [original CFI article model field: gpt-6](https://github.com/burningtree/cannabis-freedom-index/blob/8eb22ae0e9288511109383cfb6316521479008c7/articles/vanuatu-five-plants-remand-2024.md) |
| [bella-culley-tbilisi-sentence.md](../reports/2025/11/bella-culley-tbilisi-sentence.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/dd945e4015544ee52eb1280fb4de63ee300cd920) |
| [bnn-aceh-utara-sawang-69-tonnes.md](../reports/2025/11/bnn-aceh-utara-sawang-69-tonnes.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/dd945e4015544ee52eb1280fb4de63ee300cd920) |
| [singapore-hangs-omar-bin-yacob-bamadhaj.md](../reports/2026/04/singapore-hangs-omar-bin-yacob-bamadhaj.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/dd945e4015544ee52eb1280fb4de63ee300cd920) |
| [california-ucetf-q2-2026.md](../reports/2026/07/california-ucetf-q2-2026.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/dd945e4015544ee52eb1280fb4de63ee300cd920) |
| [kalinga-tinglayan-butbut-48000-plants.md](../reports/2026/08/kalinga-tinglayan-butbut-48000-plants.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/dd945e4015544ee52eb1280fb4de63ee300cd920) |
| [ndlea-edo-igbanke-farms.md](../reports/2026/08/ndlea-edo-igbanke-farms.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/dd945e4015544ee52eb1280fb4de63ee300cd920) |
| [odisha-36500-acres-2025-26.md](../reports/2026/08/odisha-36500-acres-2025-26.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/dd945e4015544ee52eb1280fb4de63ee300cd920) |
| [paraguay-nueva-alianza-57.md](../reports/2026/08/paraguay-nueva-alianza-57.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/dd945e4015544ee52eb1280fb4de63ee300cd920) |
| [bara-pacharauta-patrol-55kg.md](../reports/2026/10/bara-pacharauta-patrol-55kg.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/e44626f29771176d54d0460e57fa100e23bb067a) |
| [kaduna-expressway-musa-1027kg-skunk.md](../reports/2026/10/kaduna-expressway-musa-1027kg-skunk.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/e44626f29771176d54d0460e57fa100e23bb067a) |
| [nimta-stf-90kg-five-arrests.md](../reports/2026/10/nimta-stf-90kg-five-arrests.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/e44626f29771176d54d0460e57fa100e23bb067a) |
| [raxaul-dri-stf-514kg-charas.md](../reports/2026/10/raxaul-dri-stf-514kg-charas.md) | `anthropic/opus-5.5` | [creation commit co-author: Claude Opus 5.5](https://github.com/cannabis-freedom-commons/cet/commit/e44626f29771176d54d0460e57fa100e23bb067a) |
