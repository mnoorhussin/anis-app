/**
 * Bundle-size budget for the embed.
 *
 * A support widget is a third-party script on somebody else's storefront. If
 * it costs them Core Web Vitals they will remove it, and no feature makes up
 * for that. The budget is deliberately a build failure rather than a warning:
 * warnings about bundle size are never acted on.
 *
 * Raise the numbers only with a reason recorded in the commit message.
 */

import { gzipSync, brotliCompressSync } from 'node:zlib';
import { readFileSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const BUDGET_GZIP_KB = 45;
const BUDGET_BROTLI_KB = 40;

const bundle = fileURLToPath(new URL('../dist/widget.js', import.meta.url));

let raw;
try {
  raw = readFileSync(bundle);
} catch {
  console.error(`check-size: ${bundle} not found — run \`pnpm build\` first.`);
  process.exit(1);
}

const kb = (bytes) => Math.round((bytes / 1024) * 10) / 10;

const rawKb = kb(statSync(bundle).size);
const gzipKb = kb(gzipSync(raw, { level: 9 }).length);
const brotliKb = kb(brotliCompressSync(raw).length);

// Caddy serves this with zstd/br/gzip, so the compressed sizes are what a
// visitor actually downloads. Raw is reported because it is what the browser
// has to parse and compile, which is the cost on a low-end phone.
console.log(`widget.js  raw ${rawKb} KB · gzip ${gzipKb} KB · brotli ${brotliKb} KB`);

const failures = [];
if (gzipKb > BUDGET_GZIP_KB) failures.push(`gzip ${gzipKb} KB > ${BUDGET_GZIP_KB} KB`);
if (brotliKb > BUDGET_BROTLI_KB) failures.push(`brotli ${brotliKb} KB > ${BUDGET_BROTLI_KB} KB`);

if (failures.length > 0) {
  console.error(`\ncheck-size: over budget — ${failures.join('; ')}`);
  process.exit(1);
}
