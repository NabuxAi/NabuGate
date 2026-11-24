// Narration for the Claude Desktop tutorial. Reads each scene's `say` from
// src/guideCopy.js, asks ElevenLabs for one MP3 per scene, and writes each
// clip's length to src/guideTimings.json so a scene lasts as long as its line.
//
//   ELEVENLABS_API_KEY=… node tts.mjs          # only scenes with no clip yet
//   ELEVENLABS_API_KEY=… node tts.mjs --force  # every scene again
//
// The key is read from the environment only and is never written anywhere.
import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { GUIDE, GUIDE_LANGS } from './src/guideCopy.js';

const root = dirname(fileURLToPath(import.meta.url));
const force = process.argv.includes('--force');
const key = process.env.ELEVENLABS_API_KEY;
// "Alice", a premade multilingual voice. eleven_v3 is the model that speaks
// Persian with Persian phonology; multilingual_v2 rejects `fa`.
const voice = process.env.ELEVENLABS_VOICE || 'Xb7hH8MSUJpSbSDYk0k2';
const model = 'eleven_v3';

const timings = {};
for (const lang of GUIDE_LANGS) {
  const dir = join(root, 'public', 'guide', lang);
  mkdirSync(dir, { recursive: true });
  timings[lang] = [];
  for (const [i, scene] of GUIDE[lang].scenes.entries()) {
    const file = join(dir, `s${i + 1}.mp3`);
    if (force || !existsSync(file)) {
      if (!key) {
        console.error(`missing ${file} and no ELEVENLABS_API_KEY to make it`);
        process.exit(1);
      }
      const res = await fetch(`https://api.elevenlabs.io/v1/text-to-speech/${voice}?output_format=mp3_44100_128`, {
        method: 'POST',
        headers: { 'xi-api-key': key, 'content-type': 'application/json' },
        body: JSON.stringify({ model_id: model, language_code: lang, text: scene.say }),
      });
      if (!res.ok) {
        console.error(`${lang} s${i + 1}: ${res.status} ${await res.text()}`);
        process.exit(1);
      }
      writeFileSync(file, Buffer.from(await res.arrayBuffer()));
      console.log(`wrote ${lang}/s${i + 1}.mp3`);
    }
    const probe = spawnSync('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', file], { encoding: 'utf8' });
    const seconds = Number.parseFloat(probe.stdout);
    if (!Number.isFinite(seconds)) {
      console.error(`ffprobe could not read ${file}`);
      process.exit(1);
    }
    timings[lang].push(Number(seconds.toFixed(3)));
  }
}
writeFileSync(join(root, 'src', 'guideTimings.json'), JSON.stringify(timings, null, 2) + '\n');
console.log('wrote src/guideTimings.json');
