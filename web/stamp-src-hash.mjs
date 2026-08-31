// Records a hash of the console's build INPUTS beside the built bundle.
//
// web/dist is committed and the Dockerfile never runs npm, so a change that is
// not rebuilt produces a deploy identical to the one before it. Nothing fails:
// Go cannot see the frontend and there are no JS tests, so the change simply
// never ships. Three commits in this repo did exactly that.
//
// Two things this got wrong at first, both found by review rather than by
// failing:
//
//   - It walked the filesystem, so an untracked file (a .DS_Store) was hashed
//     locally and absent in CI. CI then failed with "run npm run build", which
//     reproduces the same wrong stamp — advice that cannot work. The file list
//     now comes from git, so both sides see the same tree.
//   - It hashed only src/, while index.html, public/ and vite.config.js are
//     equally build inputs. Editing one changed what shipped and left the
//     guard green.
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// fileURLToPath, not URL.pathname: the latter is percent-encoded on any path
// containing a space and yields /C:/... on Windows.
const root = dirname(fileURLToPath(import.meta.url));

// Tracked build inputs only, in git's own order, so Go and Node agree.
const files = execFileSync('git', ['ls-files', '-z', 'src', 'public', 'index.html', 'vite.config.js'], {
  cwd: root,
  encoding: 'buffer',
})
  .toString('utf8')
  .split('\0')
  .filter(Boolean)
  .sort();

if (files.length === 0) {
  throw new Error('git ls-files listed no build inputs under web/ — refusing to write a stamp that means nothing');
}

const hash = createHash('sha256');
for (const rel of files) {
  hash.update(rel);
  hash.update('\0');
  hash.update(readFileSync(join(root, rel)));
  hash.update('\0');
}

// The bundle the sources produced, so a stamp cannot be current while dist is
// stale or missing. index.html names the hashed asset files.
const indexHtml = readFileSync(join(root, 'dist', 'index.html'), 'utf8');
const assets = [...indexHtml.matchAll(/assets\/[A-Za-z0-9._-]+/g)].map((m) => m[0]).sort();
if (assets.length === 0) {
  throw new Error('web/dist/index.html references no assets — refusing to stamp a bundle that cannot be verified');
}
hash.update('dist:');
for (const a of assets) {
  hash.update(a);
  hash.update('\0');
}

writeFileSync(join(root, 'src.sha256'), hash.digest('hex') + '\n');
