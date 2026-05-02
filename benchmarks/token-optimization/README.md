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
| Chrome full AX tree, verbose CDP JSON | 245,351 |
| Chrome full AX tree, compact JSON | 42,832 |
| Playwright ariaSnapshot | 11,577 |
| VulpineOS optimized DOM | 3,709 |

Measured reduction:

- 98.5% fewer tokens than Chrome full AX tree as verbose CDP JSON
- 91.3% fewer tokens than Chrome full AX tree as compact JSON
- 68.0% fewer tokens than Playwright ariaSnapshot
- 70.9% fewer tokens than raw HTML

Use the generated JSON result as the source for marketing claims. Do not publish competitor-specific optimized numbers unless they are produced by this benchmark or by a linked public benchmark script.
