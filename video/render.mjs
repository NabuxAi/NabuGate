// Renders every locale to a small H.264 MP4, a VP9 WebM, and a JPEG poster.
// Output lands in web/public/intro so the Vite build (and the embedded
// gateway bundle) serves it from the same origin as the page.
import { spawnSync } from 'node:child_process';
import { mkdirSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { bundle } from '@remotion/bundler';
import { renderMedia, renderStill, selectComposition } from '@remotion/renderer';
import { COPY, LANGS } from './src/copy.js';

const root = dirname(fileURLToPath(import.meta.url));
const outDir = join(root, 'out');
const publicDir = join(root, '..', 'web', 'public', 'intro');
const chrome = process.env.CHROME || '/usr/local/bin/google-chrome';
const entry = join(root, 'src', 'index.js');

mkdirSync(outDir, { recursive: true });
mkdirSync(publicDir, { recursive: true });

if (!COPY || LANGS.length === 0) {
  console.error('no locales in src/copy.js');
  process.exit(1);
}

const chromiumOptions = {
  executablePath: chrome,
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
  gl: 'angle',
};

console.log('bundling…');
const serveUrl = await bundle({ entryPoint: entry, publicDir: join(root, 'public') });

for (const lang of LANGS) {
  const id = `intro-${lang}`;
  const raw = join(outDir, `${id}.mp4`);
  console.log(`rendering ${id}…`);
  const composition = await selectComposition({ serveUrl, id, chromiumOptions });
  await renderMedia({
    composition,
    serveUrl,
    codec: 'h264',
    outputLocation: raw,
    crf: 28,
    imageFormat: 'jpeg',
    jpegQuality: 80,
    chromiumOptions,
    concurrency: 2,
  });
  await renderStill({
    composition,
    serveUrl,
    output: join(publicDir, `nabugate-${lang}.jpg`),
    frame: 36,
    imageFormat: 'jpeg',
    jpegQuality: 80,
    chromiumOptions,
  });

  const mp4 = join(publicDir, `nabugate-${lang}.mp4`);
  const webm = join(publicDir, `nabugate-${lang}.webm`);
  // Silent audio, faststart, and a tighter encode. The picture already has
  // the captions; there is no voice and no music.
  run('ffmpeg', [
    '-y', '-i', raw,
    '-f', 'lavfi', '-i', 'anullsrc=channel_layout=stereo:sample_rate=44100',
    '-shortest',
    '-c:v', 'libx264', '-preset', 'slow', '-crf', '30', '-pix_fmt', 'yuv420p',
    '-movflags', '+faststart',
    '-c:a', 'aac', '-b:a', '32k',
    mp4,
  ]);
  run('ffmpeg', [
    '-y', '-i', mp4,
    '-an',
    '-c:v', 'libvpx-vp9', '-crf', '38', '-b:v', '0', '-cpu-used', '4', '-row-mt', '1',
    '-deadline', 'good',
    webm,
  ]);
  for (const file of [mp4, webm, join(publicDir, `nabugate-${lang}.jpg`)]) {
    const kb = statSync(file).size / 1024;
    console.log(`  ${file}  ${kb.toFixed(0)} KiB`);
  }
}

function run(cmd, args) {
  const res = spawnSync(cmd, args, { stdio: 'inherit' });
  if (res.status !== 0) {
    console.error(`${cmd} failed`);
    process.exit(res.status || 1);
  }
}
