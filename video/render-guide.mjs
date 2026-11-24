// Renders the Claude Desktop tutorial (src/Guide.jsx) for every language to an
// H.264 MP4 with its narration and a JPEG poster, into web/public/guide so the
// docs page serves them from its own origin (and so from the Iran mirror too).
// Run `node tts.mjs` first if the narration changed.
import { spawnSync } from 'node:child_process';
import { mkdirSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { bundle } from '@remotion/bundler';
import { renderMedia, renderStill, selectComposition } from '@remotion/renderer';
import { GUIDE_LANGS } from './src/guideCopy.js';

const root = dirname(fileURLToPath(import.meta.url));
const outDir = join(root, 'out');
const publicDir = join(root, '..', 'web', 'public', 'guide');
const chrome = process.env.CHROME || '/usr/local/bin/google-chrome';
const only = process.argv.slice(2).filter((a) => GUIDE_LANGS.includes(a));
mkdirSync(outDir, { recursive: true });
mkdirSync(publicDir, { recursive: true });

// The browser is a top-level option; Remotion ignores an executablePath inside
// chromiumOptions and falls back to downloading its own headless shell.
const browserExecutable = chrome;
const chromiumOptions = { gl: 'angle' };
console.log('bundling…');
const serveUrl = await bundle({ entryPoint: join(root, 'src', 'index.js'), publicDir: join(root, 'public') });

for (const lang of only.length ? only : GUIDE_LANGS) {
  const id = `guide-${lang}`;
  const raw = join(outDir, `${id}.mp4`);
  console.log(`rendering ${id}…`);
  const composition = await selectComposition({ serveUrl, id, browserExecutable, chromiumOptions, inputProps: { lang } });
  await renderMedia({ composition, serveUrl, codec: 'h264', outputLocation: raw, crf: 24, imageFormat: 'jpeg', jpegQuality: 85, browserExecutable, chromiumOptions, concurrency: 4, inputProps: { lang } });
  const poster = join(publicDir, `claude-desktop-${lang}.jpg`);
  await renderStill({ composition, serveUrl, output: poster, frame: 40, imageFormat: 'jpeg', jpegQuality: 80, browserExecutable, chromiumOptions, inputProps: { lang } });
  const mp4 = join(publicDir, `claude-desktop-${lang}.mp4`);
  // Mostly still frames: a slow preset and a high CRF keep a two-minute
  // tutorial small enough to embed in the gateway binary.
  run('ffmpeg', ['-y', '-i', raw, '-c:v', 'libx264', '-preset', 'slow', '-crf', '30', '-tune', 'stillimage', '-pix_fmt', 'yuv420p',
    '-movflags', '+faststart', '-c:a', 'aac', '-b:a', '64k', '-ac', '1', mp4]);
  for (const file of [mp4, poster]) console.log(`  ${file}  ${(statSync(file).size / 1024).toFixed(0)} KiB`);
}

function run(cmd, args) {
  const res = spawnSync(cmd, args, { stdio: 'inherit' });
  if (res.status !== 0) {
    console.error(`${cmd} failed`);
    process.exit(res.status || 1);
  }
}
