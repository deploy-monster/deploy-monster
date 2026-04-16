// Bundle-size gate. Runs after `pnpm run build` and fails CI if the
// main entry chunk exceeds the budget. The budget is intentionally
// generous (300 KB gzipped) relative to the current ~8 KB baseline —
// it's a "don't regress catastrophically" guard, not an optimization
// pressure point.
//
// Scope: only the main entry chunk (dist/assets/index-*.js) is gated.
// Route-level lazy chunks (Topology, AppDetail, etc.) and vendor
// splits (vendor-react, vendor-graph, vendor-ui) are intentionally
// excluded — they're the output of the deliberate code-splitting
// strategy in vite.config.ts and should be measured separately if the
// need arises. A bundle-size regression on the main chunk means
// something that should have been lazy is now eagerly imported, which
// is the actual thing worth catching.

import { readdirSync, readFileSync } from 'node:fs';
import { gzipSync } from 'node:zlib';
import { resolve, join } from 'node:path';

const MAX_MAIN_CHUNK_GZIP = 300 * 1024; // 300 KB
const distDir = resolve(process.cwd(), 'dist');
const assetsDir = join(distDir, 'assets');

let mainChunk;
try {
  mainChunk = readdirSync(assetsDir).find((f) => f.startsWith('index-') && f.endsWith('.js'));
} catch (err) {
  console.error(`bundle-size: could not read ${assetsDir} — did you run 'pnpm run build' first?`);
  console.error(err.message);
  process.exit(1);
}

if (!mainChunk) {
  console.error(`bundle-size: no index-*.js chunk found in ${assetsDir}`);
  process.exit(1);
}

const raw = readFileSync(join(assetsDir, mainChunk));
const gzipped = gzipSync(raw);
const gzipKB = (gzipped.length / 1024).toFixed(2);
const budgetKB = (MAX_MAIN_CHUNK_GZIP / 1024).toFixed(0);

if (gzipped.length > MAX_MAIN_CHUNK_GZIP) {
  console.error(
    `bundle-size: FAIL — main chunk ${mainChunk} is ${gzipKB} KB gzipped (budget ${budgetKB} KB).`,
  );
  console.error(
    `           Something heavy was pulled into the entry bundle. Check vite.config.ts manualChunks and App.tsx lazy() imports.`,
  );
  process.exit(1);
}

console.log(
  `bundle-size: OK — main chunk ${mainChunk} is ${gzipKB} KB gzipped (budget ${budgetKB} KB).`,
);
