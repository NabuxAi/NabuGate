// Writes Brotli and gzip copies beside every text file in dist/, which the
// gateway serves to clients that accept them (internal/server/static.go).
//
// Compressing here rather than per request costs nothing at serve time and
// lets both use their slowest, smallest setting: Brotli 11 is too slow to run
// per request but is about a fifth smaller than the gzip the edge proxy
// produces on the fly. Fonts (woff2) are already compressed and are skipped.
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { brotliCompressSync, constants, gzipSync } from 'node:zlib';

const dist = join(dirname(fileURLToPath(import.meta.url)), 'dist');
const TEXT = /\.(js|css|html|svg|json|txt|map)$/;
// Below this a compressed response saves less than its own headers cost.
const MIN = 1024;

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* walk(p);
    else yield p;
  }
}

let n = 0, raw = 0, br = 0;
for (const file of walk(dist)) {
  if (!TEXT.test(file)) continue;
  const body = readFileSync(file);
  if (body.length < MIN) continue;
  const b = brotliCompressSync(body, {
    params: {
      [constants.BROTLI_PARAM_QUALITY]: constants.BROTLI_MAX_QUALITY,
      [constants.BROTLI_PARAM_SIZE_HINT]: body.length,
    },
  });
  const g = gzipSync(body, { level: 9 });
  // A copy that is not smaller is not worth a second representation.
  if (b.length < body.length) writeFileSync(file + '.br', b);
  if (g.length < body.length) writeFileSync(file + '.gz', g);
  n++; raw += body.length; br += b.length;
}
console.log(`compressed ${n} files: ${(raw / 1024).toFixed(0)} KiB → ${(br / 1024).toFixed(0)} KiB brotli`);
