// Records a hash of web/src beside the built bundle.
//
// web/dist is committed and the Dockerfile never runs npm, so a change under
// web/src that is not rebuilt produces a deploy identical to the one before it.
// Nothing fails: the Go toolchain cannot see the frontend and there are no JS
// tests, so the change simply never ships. Three commits in this repo's history
// did exactly that — two dark-mode colour fixes and a CSS typo — and were live
// only once somebody happened to rebuild for an unrelated reason.
//
// The hash is written outside dist so it is neither embedded nor served, and
// TestBundleWasBuiltFromTheCurrentSources compares it to a freshly computed one.
import { createHash } from 'node:crypto';
import { readdirSync, readFileSync, writeFileSync, statSync } from 'node:fs';
import { join, relative, sep } from 'node:path';

const root = new URL('.', import.meta.url).pathname;
const srcDir = join(root, 'src');

function walk(dir, out = []) {
  for (const entry of readdirSync(dir).sort()) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) walk(full, out);
    else out.push(full);
  }
  return out;
}

const hash = createHash('sha256');
for (const file of walk(srcDir)) {
  // Paths are hashed with forward slashes so the value does not depend on the
  // platform the bundle happened to be built on.
  hash.update(relative(srcDir, file).split(sep).join('/'));
  hash.update('\0');
  hash.update(readFileSync(file));
  hash.update('\0');
}

writeFileSync(join(root, 'src.sha256'), hash.digest('hex') + '\n');
