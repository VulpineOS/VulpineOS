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
The generated JSON normalizes local runner metadata so committed results do not expose a developer machine's OS, Node version, or executable path.

## Published Results

Use the generated JSON output as the source of truth for any public claim. Do not hand-copy local numbers into docs; regenerate the artifact and review it with the release diff.

The benchmark fails by default if the optimized export drops required semantic strings or falls below the fixture coverage checks. Use `--no-fail-on-quality` only when exploring non-release profiles.

Runtime defaults use the same compact profile family as the benchmark. Agents and MCP callers can opt into larger profiles when needed:

- `compact`: default context-saving snapshot
- `expanded`: larger snapshot for retry paths
- `full`: broad inspection profile for difficult pages

If a compact snapshot is truncated and a target may have been pruned, callers should retry with `retry:true` or `profile:"expanded"` before concluding the target is absent.
