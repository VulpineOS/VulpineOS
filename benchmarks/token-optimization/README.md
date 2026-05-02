# Token Optimization Benchmark

Reproducible benchmark for the VulpineOS token-optimized DOM export.

The benchmark renders deterministic local fixtures in Chrome, then counts GPT tokenizer tokens for:

- raw page HTML
- Chrome full accessibility tree from `Accessibility.getFullAXTree`
- Playwright `ariaSnapshot`
- VulpineOS-style optimized DOM JSON

Run:

```bash
npm run benchmark:tokens
```

Optional:

```bash
CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" npm run benchmark:tokens
npm run benchmark:tokens -- --output benchmarks/token-optimization/results/latest.json
```

The fixture pages are synthetic but deterministic, so published numbers can be reproduced without scraping live ecommerce sites or depending on third-party pages changing underneath us.

## Current Local Result

Last run on 2026-05-02 with Google Chrome via Playwright Core:

| Metric | Mean tokens |
|---|---:|
| Raw HTML | 12,761 |
| Chrome full AX tree, verbose CDP JSON | 245,352 |
| Chrome full AX tree, compact JSON | 42,832 |
| Playwright ariaSnapshot | 11,577 |
| VulpineOS optimized DOM | 2,942 |

Measured reduction:

- 98.8% fewer tokens than Chrome full AX tree as verbose CDP JSON
- 93.1% fewer tokens than Chrome full AX tree as compact JSON
- 74.6% fewer tokens than Playwright ariaSnapshot
- 76.9% fewer tokens than raw HTML

The benchmark fails by default if the optimized export drops required semantic strings or falls below minimum reference/heading coverage for the fixture set. Use `--no-fail-on-quality` only when exploring lower-quality profiles.

Use the generated JSON result as the source for marketing claims. Do not publish competitor-specific optimized numbers unless they are produced by this benchmark or by a linked public benchmark script.
