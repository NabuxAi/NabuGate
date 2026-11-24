import React, { useEffect, useState } from 'react';
import {
  AbsoluteFill,
  Audio,
  Easing,
  Sequence,
  continueRender,
  delayRender,
  interpolate,
  staticFile,
  useCurrentFrame,
} from 'remotion';
import { FONT_FACE, Logo } from './Intro.jsx';
import { GUIDE, GUIDE_FPS, UI } from './guideCopy.js';
import TIMINGS from './guideTimings.json';

// A scene lasts as long as its narration, plus a short breath before the next
// line starts. The voice starts a few frames after the picture does.
const LEAD = 6;
const TAIL = Math.round(0.8 * GUIDE_FPS);

export function guideScenes(lang) {
  const secs = TIMINGS[lang] || [];
  let from = 0;
  return GUIDE[lang].scenes.map((scene, i) => {
    const audio = Math.ceil((secs[i] || 6) * GUIDE_FPS);
    const length = LEAD + audio + TAIL;
    const out = { ...scene, from, length, audio, index: i };
    from += length;
    return out;
  });
}

export function guideDuration(lang) {
  const scenes = guideScenes(lang);
  const last = scenes[scenes.length - 1];
  return last.from + last.length;
}

const ease = Easing.bezier(0.16, 1, 0.3, 1);

function appear(local, delay, span = 14) {
  return interpolate(local, [delay, delay + span], [0, 1], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: ease });
}

function rise(local, delay) {
  const t = appear(local, delay, 16);
  return { opacity: t, transform: `translateY(${(1 - t) * 16}px)` };
}

const MONO = '"JetBrains Mono", Consolas, "Cascadia Mono", monospace';
const LATIN = 'Inter, Vazirmatn, sans-serif';

// A neutral app window. It is not a picture of Claude Desktop: only the menu
// and field labels are the app's, so a viewer knows what to look for.
function Window({ title, children, local, width = 600 }) {
  return (
    <div dir="ltr" style={{
      ...rise(local, 4),
      width, borderRadius: 16, overflow: 'hidden',
      background: '#0d1426', border: '1px solid rgba(148,163,184,0.22)',
      boxShadow: '0 30px 80px rgba(2,6,23,0.6)', fontFamily: LATIN,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '12px 16px', background: '#111a31', borderBottom: '1px solid rgba(148,163,184,0.14)' }}>
        <i style={dot('#f87171')} /><i style={dot('#fbbf24')} /><i style={dot('#34d399')} />
        <span style={{ marginLeft: 10, color: '#9fb0cf', fontSize: 17, fontWeight: 600 }}>{title}</span>
      </div>
      <div style={{ padding: 22 }}>{children}</div>
    </div>
  );
}

function dot(color) {
  return { width: 11, height: 11, borderRadius: 99, background: color, display: 'inline-block' };
}

// One menu level. `hot` is when this item lights up, as the narration names it.
function MenuItem({ label, local, hot, depth = 0 }) {
  const lit = appear(local, hot, 10);
  return (
    <div style={{
      opacity: 0.25 + 0.75 * appear(local, 8 + depth * 8, 10),
      marginLeft: depth * 38, marginTop: 10,
      display: 'flex', alignItems: 'center', gap: 12,
      padding: '12px 16px', borderRadius: 10, fontSize: 22, fontWeight: 600,
      color: lit > 0.5 ? '#fff' : '#c8d3ea',
      background: `rgba(91,124,255,${0.08 + lit * 0.32})`,
      border: `1px solid rgba(138,166,255,${0.15 + lit * 0.5})`,
    }}>
      <span style={{ color: '#8aa6ff' }}>{depth === 0 ? '☰' : '▸'}</span>
      {label}
    </div>
  );
}

function Field({ label, value, local, at, highlight }) {
  const shown = appear(local, at, 12);
  const typed = Math.round(interpolate(local, [at + 4, at + 26], [0, value.length], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' }));
  return (
    <div style={{ display: 'grid', gridTemplateColumns: '210px 1fr', alignItems: 'center', gap: 14, marginTop: 12, opacity: 0.35 + shown * 0.65 }}>
      <span style={{ color: '#9fb0cf', fontSize: 17, fontWeight: 600 }}>{label}</span>
      <span style={{
        fontFamily: MONO, fontSize: 18, color: '#e8eefc', padding: '10px 12px', borderRadius: 8,
        background: '#0a1020', border: `1px solid ${highlight && shown > 0.5 ? 'rgba(16,185,129,0.7)' : 'rgba(148,163,184,0.25)'}`,
        whiteSpace: 'nowrap', overflow: 'hidden', minHeight: 24,
      }}>
        {value.slice(0, typed)}
      </span>
    </div>
  );
}

function Button({ label, local, at, primary }) {
  const press = interpolate(local, [at, at + 5, at + 12], [1, 0.94, 1], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' });
  const lit = appear(local, at, 8);
  return (
    <span style={{
      display: 'inline-flex', padding: '14px 24px', borderRadius: 10, fontSize: 21, fontWeight: 700,
      transform: `scale(${press})`,
      color: primary ? '#fff' : '#dbe4ff',
      background: primary ? `rgba(91,124,255,${0.45 + lit * 0.5})` : 'rgba(148,163,184,0.12)',
      border: `1px solid rgba(138,166,255,${0.3 + lit * 0.6})`,
      boxShadow: lit > 0.5 ? '0 0 0 6px rgba(91,124,255,0.18)' : 'none',
    }}>{label}</span>
  );
}

function Visual({ scene, local, lang }) {
  const span = scene.audio;
  switch (scene.visual) {
    case 'intro':
      return (
        <div dir="ltr" style={{ display: 'flex', alignItems: 'center', gap: 18, fontFamily: LATIN }}>
          {['Claude Desktop', 'NabuGate', 'Claude'].map((name, i) => (
            <React.Fragment key={name}>
              {i > 0 && <span style={{ ...rise(local, 10 + i * 14), fontSize: 34, color: '#8aa6ff' }}>→</span>}
              <span style={{
                ...rise(local, 6 + i * 14), padding: '22px 26px', borderRadius: 16, fontSize: 24, fontWeight: 750,
                background: i === 1 ? 'linear-gradient(135deg, rgba(79,140,255,0.35), rgba(168,85,247,0.35))' : 'rgba(148,163,184,0.10)',
                border: `1px solid ${i === 1 ? 'rgba(138,166,255,0.7)' : 'rgba(148,163,184,0.25)'}`,
              }}>{name}</span>
            </React.Fragment>
          ))}
        </div>
      );
    case 'key':
      return (
        <Window title="NabuGate · API keys" local={local}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ color: '#c8d3ea', fontSize: 20, fontWeight: 700 }}>API keys</span>
            <Button label="+ New key" local={local} at={Math.round(span * 0.3)} primary />
          </div>
          <div style={{ ...rise(local, Math.round(span * 0.45)), marginTop: 22, display: 'flex', alignItems: 'center', gap: 12, padding: 14, borderRadius: 10, background: '#0a1020', border: '1px solid rgba(16,185,129,0.5)' }}>
            <span style={{ fontFamily: MONO, fontSize: 19, color: '#bbf7d0', flex: 1 }}>ng-3f9c••••••••••••••••a71e</span>
            <Button label="Copy" local={local} at={Math.round(span * 0.75)} />
          </div>
        </Window>
      );
    case 'devmode':
      return (
        <Window title="Claude Desktop" local={local} width={560}>
          {UI.menu.map((label, i) => (
            <MenuItem key={label} label={label} local={local} depth={i} hot={Math.round(span * (0.45 + i * 0.17))} />
          ))}
        </Window>
      );
    case 'developer':
      return (
        <Window title="Claude Desktop" local={local} width={560}>
          {UI.dev.map((label, i) => (
            <MenuItem key={label} label={label} local={local} depth={i} hot={Math.round(span * (0.15 + i * 0.4))} />
          ))}
        </Window>
      );
    case 'connection':
      return (
        <Window title="Configure Third-Party Inference · Connection" local={local} width={640}>
          {UI.fields.map(([label, value], i) => (
            <Field key={label} label={label} value={value} local={local} at={Math.round(span * (0.12 + i * 0.13))} highlight={i === 1} />
          ))}
          <div style={{ ...rise(local, Math.round(span * 0.82)), marginTop: 16, display: 'flex', gap: 10, alignItems: 'center', fontSize: 17, color: '#fde68a' }}>
            <span style={{ padding: '4px 10px', borderRadius: 6, background: 'rgba(251,191,36,0.14)', border: '1px solid rgba(251,191,36,0.45)', fontWeight: 700 }}>IR</span>
            <span style={{ fontFamily: MONO }}>{UI.iranUrl}</span>
          </div>
        </Window>
      );
    case 'models':
      return (
        <Window title="Configure Third-Party Inference · Models" local={local} width={560}>
          {UI.models.map((id, i) => (
            <div key={id} style={{ ...rise(local, Math.round(span * (0.06 + i * 0.12))), display: 'flex', alignItems: 'center', gap: 12, marginTop: 12, padding: '12px 14px', borderRadius: 10, background: '#0a1020', border: '1px solid rgba(148,163,184,0.25)' }}>
              <span style={{ color: '#34d399', fontSize: 20 }}>＋</span>
              <span style={{ fontFamily: MONO, fontSize: 21, color: '#e8eefc' }}>{id}</span>
            </div>
          ))}
        </Window>
      );
    case 'apply':
      return (
        <Window title="Configure Third-Party Inference" local={local} width={560}>
          <div style={{ display: 'flex', gap: 14, justifyContent: 'flex-end' }}>
            <Button label={UI.apply[0]} local={local} at={Math.round(span * 0.12)} />
            <Button label={UI.apply[1]} local={local} at={Math.round(span * 0.3)} primary />
          </div>
          <div style={{ ...rise(local, Math.round(span * 0.55)), marginTop: 24, padding: 16, borderRadius: 10, background: 'rgba(16,185,129,0.10)', border: '1px solid rgba(16,185,129,0.45)', color: '#bbf7d0', fontSize: 19, fontWeight: 600 }}>
            ✓ Gateway · gate.nabuxai.com
          </div>
        </Window>
      );
    case 'usage': {
      const rows = [['claude-sonnet-5-5', '1,284', '$0.0071'], ['claude-haiku-5-5', '412', '$0.0006'], ['claude-opus-5-5', '2,950', '$0.0410']];
      return (
        <Window title="NabuGate · Usage" local={local} width={620}>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 110px 110px', gap: 10, color: '#7f8fb0', fontSize: 15, fontWeight: 700, textTransform: 'uppercase', letterSpacing: 0.5 }}>
            <span>model</span><span>tokens</span><span>cost</span>
          </div>
          {rows.map((r, i) => (
            <div key={r[0]} style={{ ...rise(local, Math.round(span * (0.03 + i * 0.05))), display: 'grid', gridTemplateColumns: '1fr 110px 110px', gap: 10, marginTop: 10, padding: '10px 0', borderTop: '1px solid rgba(148,163,184,0.14)', fontFamily: MONO, fontSize: 18, color: '#e8eefc' }}>
              <span>{r[0]}</span><span>{r[1]}</span><span style={{ color: '#bbf7d0' }}>{r[2]}</span>
            </div>
          ))}
        </Window>
      );
    }
    default:
      return null;
  }
}

export const Guide = ({ lang = 'fa' }) => {
  const copy = GUIDE[lang] || GUIDE.fa;
  const scenes = guideScenes(lang);
  const frame = useCurrentFrame();
  const [handle] = useState(() => delayRender(`guide-fonts-${lang}`));

  useEffect(() => {
    const id = 'nabu-intro-fonts';
    if (!document.getElementById(id)) {
      const style = document.createElement('style');
      style.id = id;
      style.textContent = FONT_FACE;
      document.head.appendChild(style);
    }
    let cancel = false;
    document.fonts.ready.then(() => { if (!cancel) continueRender(handle); });
    return () => { cancel = true; };
  }, [handle]);

  const current = scenes.findLast((s) => frame >= s.from) || scenes[0];
  const rtl = copy.dir === 'rtl';
  const step = current.index;

  return (
    <AbsoluteFill style={{ background: '#070b16', color: '#f4f6fb', fontFamily: `${copy.font}, Vazirmatn, Inter, sans-serif`, overflow: 'hidden' }}>
      <AbsoluteFill style={{ background: 'radial-gradient(900px 480px at 88% -10%, rgba(91,124,255,0.26), transparent 60%), radial-gradient(700px 420px at 0% 110%, rgba(168,85,247,0.2), transparent 60%)' }} />
      <AbsoluteFill style={{
        backgroundImage: 'linear-gradient(rgba(148,163,184,0.06) 1px, transparent 1px), linear-gradient(90deg, rgba(148,163,184,0.06) 1px, transparent 1px)',
        backgroundSize: '56px 56px',
        maskImage: 'radial-gradient(ellipse 80% 70% at 50% 40%, #000 20%, transparent 75%)',
        WebkitMaskImage: 'radial-gradient(ellipse 80% 70% at 50% 40%, #000 20%, transparent 75%)',
      }} />

      {scenes.map((s) => (
        <Sequence key={`a${s.index}`} from={s.from + LEAD} durationInFrames={s.audio} layout="none">
          <Audio src={staticFile(`guide/${lang}/s${s.index + 1}.mp3`)} />
        </Sequence>
      ))}

      <div style={{ position: 'absolute', top: 32, insetInline: 56, display: 'flex', alignItems: 'center', gap: 14, direction: copy.dir }}>
        <Logo />
        <span style={{ fontWeight: 800, fontSize: 24, fontFamily: LATIN }}>NabuGate</span>
        <span style={{ marginInlineStart: 'auto', display: 'flex', gap: 8 }}>
          {scenes.map((s) => (
            <i key={s.index} style={{ width: s.index === step ? 28 : 8, height: 8, borderRadius: 99, background: s.index <= step ? '#8aa6ff' : 'rgba(148,163,184,0.35)' }} />
          ))}
        </span>
      </div>

      {scenes.map((s) => (
        <Sequence key={s.index} from={s.from} durationInFrames={s.length} layout="none">
          <Scene scene={s} copy={copy} rtl={rtl} lang={lang} />
        </Sequence>
      ))}
    </AbsoluteFill>
  );
};

// In right-to-left text a run like "/v1" or "Apply Changes → Save" would be
// reordered by the bidi algorithm ("v1/"). Each Latin run is isolated as LTR.
const LATIN_RUN = /([/A-Za-z0-9][\w./:&…'-]*(?:\s+[/A-Za-z0-9&][\w./:&…'-]*)*)/g;
function isolateLatin(text) {
  return text.split(LATIN_RUN).flatMap((part, i) => {
    if (i % 2 === 0) return [part];
    // A sentence's full stop belongs to the RTL sentence, not to the run.
    const stop = part.match(/[.…]+$/);
    const run = stop ? part.slice(0, -stop[0].length) : part;
    return [<bdi key={i} dir="ltr">{run}</bdi>, stop ? stop[0] : ''];
  });
}

function Scene({ scene, copy, rtl, lang }) {
  const local = useCurrentFrame();
  const fade = Math.min(
    interpolate(local, [0, 12], [0, 1], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' }),
    interpolate(local, [scene.length - 10, scene.length], [1, 0], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' }),
  );
  return (
    <div style={{
      position: 'absolute', top: 104, bottom: 44, insetInline: 56,
      display: 'flex', alignItems: 'center', gap: 44, direction: copy.dir, opacity: fade,
    }}>
      <div style={{ flex: '0 0 470px', display: 'flex', flexDirection: 'column', gap: 18, textAlign: 'start' }}>
        <div style={{ ...rise(local, 0), alignSelf: 'flex-start', padding: '6px 14px', borderRadius: 99, background: 'rgba(91,124,255,0.16)', border: '1px solid rgba(91,124,255,0.45)', color: '#c7d2fe', fontWeight: 700, fontSize: 20 }}>
          {scene.kicker}
        </div>
        <h1 style={{ ...rise(local, 4), margin: 0, fontSize: rtl ? 48 : 50, lineHeight: rtl ? 1.35 : 1.1, fontWeight: 800, letterSpacing: rtl ? 0 : -1 }}>
          {scene.title}
        </h1>
        <p style={{ ...rise(local, 10), margin: 0, color: '#b7c3d9', fontSize: rtl ? 25 : 23, lineHeight: rtl ? 1.75 : 1.5, fontWeight: 500 }}>
          {rtl ? isolateLatin(scene.body) : scene.body}
        </p>
      </div>
      <div style={{ flex: 1, display: 'flex', justifyContent: 'center' }}>
        <Visual scene={scene} local={local} lang={lang} />
      </div>
    </div>
  );
}
