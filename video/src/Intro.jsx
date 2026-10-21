import React, { useEffect, useState } from 'react';
import {
  AbsoluteFill,
  continueRender,
  delayRender,
  interpolate,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from 'remotion';
import { COPY, SCENE_COUNT, SCENE_FRAMES } from './copy.js';

const FONT_FACE = `
@font-face { font-family: 'Vazirmatn'; src: url('${staticFile('fonts/vazirmatn-arabic.woff2')}') format('woff2'); font-weight: 100 900; font-display: block; unicode-range: U+0600-06FF, U+0750-077F, U+200C-200E, U+FB50-FDFF, U+FE70-FEFC; }
@font-face { font-family: 'Vazirmatn'; src: url('${staticFile('fonts/vazirmatn-latin.woff2')}') format('woff2'); font-weight: 100 900; font-display: block; unicode-range: U+0000-00FF, U+0131, U+0152-0153, U+02BB-02BC, U+2000-206F; }
@font-face { font-family: 'Inter'; src: url('${staticFile('fonts/inter-latin.woff2')}') format('woff2'); font-weight: 100 900; font-display: block; }
@font-face { font-family: 'Noto Naskh Arabic'; src: url('${staticFile('fonts/noto-naskh-arabic.woff2')}') format('woff2'); font-weight: 100 900; font-display: block; unicode-range: U+0600-06FF, U+0750-077F, U+200C-200E, U+FB50-FDFF, U+FE70-FEFC; }
@font-face { font-family: 'Noto Naskh Arabic'; src: url('${staticFile('fonts/noto-naskh-latin.woff2')}') format('woff2'); font-weight: 100 900; font-display: block; unicode-range: U+0000-00FF, U+0131, U+0152-0153, U+02BB-02BC, U+2000-206F; }
`;

const IRAN = ['AvalAI', 'GapGPT', 'ArvanCloud'];
const GLOBAL = ['OpenAI', 'Anthropic', 'Gemini', 'Groq', 'OpenRouter', 'Cloudflare', 'TokenRouter'];

function clampFade(frame, start, end) {
  const enter = interpolate(frame, [start, start + 14], [0, 1], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' });
  const exit = interpolate(frame, [end - 12, end], [1, 0], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' });
  return Math.min(enter, exit);
}

function rise(local, delay) {
  const t = interpolate(local, [delay, delay + 16], [0, 1], { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' });
  return { opacity: t, transform: `translateY(${(1 - t) * 18}px)` };
}

function Logo() {
  return (
    <svg width="42" height="42" viewBox="0 0 40 40" fill="none" aria-hidden="true">
      <rect x="1" y="1" width="38" height="38" rx="11" fill="url(#g)" />
      <circle cx="13" cy="13" r="2.6" fill="#fff" />
      <circle cx="13" cy="27" r="2.6" fill="#fff" />
      <circle cx="28" cy="20" r="3.4" fill="#fff" />
      <path d="M15.3 14.1 25.2 18.7M15.3 25.9 25.2 21.3" stroke="#fff" strokeWidth="1.5" strokeLinecap="round" />
      <defs>
        <linearGradient id="g" x1="0" y1="0" x2="40" y2="40" gradientUnits="userSpaceOnUse">
          <stop stopColor="#4f8cff" />
          <stop offset="0.55" stopColor="#6d5dfc" />
          <stop offset="1" stopColor="#a855f7" />
        </linearGradient>
      </defs>
    </svg>
  );
}

function Chip({ children, delay, local, tone = 'plain' }) {
  const style = rise(local, delay);
  const tones = {
    plain: { background: 'rgba(148,163,184,0.10)', color: '#e8eefc', border: '1px solid rgba(148,163,184,0.22)' },
    accent: { background: 'rgba(91,124,255,0.16)', color: '#dbe4ff', border: '1px solid rgba(91,124,255,0.45)' },
    ok: { background: 'rgba(16,185,129,0.14)', color: '#bbf7d0', border: '1px solid rgba(16,185,129,0.4)' },
    bad: { background: 'rgba(248,113,113,0.12)', color: '#fecaca', border: '1px solid rgba(248,113,113,0.4)' },
  };
  return (
    <span
      dir="ltr"
      style={{
        ...style,
        ...tones[tone],
        display: 'inline-flex',
        alignItems: 'center',
        padding: '10px 16px',
        borderRadius: 12,
        fontFamily: 'Inter, Vazirmatn, sans-serif',
        fontWeight: 650,
        fontSize: 22,
        letterSpacing: -0.01,
      }}
    >
      {children}
    </span>
  );
}

function Visual({ kind, local, copy }) {
  if (kind === 'endpoint') {
    return (
      <div dir="ltr" style={{ ...rise(local, 8), width: '100%', background: 'rgba(8,12,24,0.85)', border: '1px solid rgba(148,163,184,0.22)', borderRadius: 18, overflow: 'hidden', boxShadow: '0 30px 60px -30px rgba(91,124,255,0.55)' }}>
        <div style={{ display: 'flex', gap: 8, padding: '14px 18px', borderBottom: '1px solid rgba(148,163,184,0.14)' }}>
          <i style={dot('#ef4444')} /><i style={dot('#f59e0b')} /><i style={dot('#22c55e')} />
          <span style={{ marginInlineStart: 10, color: '#9aa8c3', fontFamily: 'Inter, sans-serif', fontSize: 18 }}>POST /v1/chat/completions</span>
        </div>
        <pre style={{ margin: 0, padding: '22px 22px 26px', color: '#dbe4ff', fontFamily: 'Inter, sans-serif', fontSize: 26, lineHeight: 1.55 }}>
          <span style={{ color: '#8aa6ff' }}>model</span>
          {': '}
          <span style={{ color: '#c4b5fd' }}>"nabu-smart"</span>
        </pre>
      </div>
    );
  }
  if (kind === 'aliases') {
    return (
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12 }}>
        {['nabu-fast', 'nabu-smart', 'nabu-embed'].map((name, i) => (
          <Chip key={name} local={local} delay={6 + i * 8} tone={i === 1 ? 'accent' : 'plain'}>{name}</Chip>
        ))}
      </div>
    );
  }
  if (kind === 'route') {
    const showSecond = local > 36;
    return (
      <div dir="ltr" style={{ width: '100%', display: 'flex', flexDirection: 'column', gap: 12 }}>
        <div style={{ ...rise(local, 4), display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '16px 18px', borderRadius: 14, background: 'rgba(248,113,113,0.08)', border: '1px solid rgba(248,113,113,0.35)', fontFamily: 'Inter, sans-serif' }}>
          <span style={{ color: '#fecaca', fontSize: 24 }}>openai</span>
          <span style={{ color: '#fca5a5', fontWeight: 700, fontSize: 22 }}>429</span>
        </div>
        <div style={{ ...rise(local, showSecond ? 28 : 80), display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '16px 18px', borderRadius: 14, background: 'rgba(16,185,129,0.10)', border: '1px solid rgba(16,185,129,0.4)', fontFamily: 'Inter, sans-serif' }}>
          <span style={{ color: '#bbf7d0', fontSize: 24 }}>anthropic</span>
          <span style={{ color: '#6ee7b7', fontWeight: 700, fontSize: 22 }}>200</span>
        </div>
      </div>
    );
  }
  if (kind === 'usage') {
    return (
      <div dir="ltr" style={{ ...rise(local, 8), width: '100%', padding: '22px 24px', borderRadius: 18, background: 'rgba(8,12,24,0.85)', border: '1px solid rgba(148,163,184,0.22)', fontFamily: 'Inter, sans-serif' }}>
        <div style={{ color: '#8aa6ff', fontSize: 22, marginBottom: 10 }}>GET /v1/usage</div>
        <div style={{ color: '#cdd6e4', fontSize: 24, lineHeight: 1.6 }}>per project · per model</div>
      </div>
    );
  }
  if (kind === 'providers') {
    return (
      <div style={{ width: '100%', display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div dir="ltr" style={{ ...rise(local, 2), fontFamily: 'Inter, sans-serif', color: '#c4b5fd', fontSize: 22 }}>X-Nabu-Key-&lt;provider&gt;</div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10 }}>
          {IRAN.map((name, i) => <Chip key={name} local={local} delay={8 + i * 4} tone="accent">{name}</Chip>)}
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10 }}>
          {GLOBAL.map((name, i) => <Chip key={name} local={local} delay={22 + i * 3}>{name}</Chip>)}
        </div>
      </div>
    );
  }
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12 }}>
      {copy.modalities.map((name, i) => (
        <Chip key={name} local={local} delay={4 + i * 5} tone={i === copy.modalities.length - 1 ? 'ok' : 'plain'}>
          <span dir={copy.dir} style={{ fontFamily: copy.font }}>{name}</span>
        </Chip>
      ))}
    </div>
  );
}

function dot(color) {
  return { width: 12, height: 12, borderRadius: 99, background: color, display: 'inline-block' };
}

export const Intro = ({ lang = 'en' }) => {
  const copy = COPY[lang] || COPY.en;
  const frame = useCurrentFrame();
  const { durationInFrames } = useVideoConfig();
  const [handle] = useState(() => delayRender(`fonts-${lang}`));

  useEffect(() => {
    const id = 'nabu-intro-fonts';
    if (!document.getElementById(id)) {
      const style = document.createElement('style');
      style.id = id;
      style.textContent = FONT_FACE;
      document.head.appendChild(style);
    }
    let cancel = false;
    document.fonts.ready.then(() => {
      if (!cancel) continueRender(handle);
    });
    return () => { cancel = true; };
  }, [handle]);

  const scene = Math.min(SCENE_COUNT - 1, Math.floor(frame / SCENE_FRAMES));
  const start = scene * SCENE_FRAMES;
  const end = Math.min(durationInFrames, start + SCENE_FRAMES);
  const local = frame - start;
  const opacity = clampFade(frame, start, end);
  const data = copy.scenes[scene];
  const rtl = copy.dir === 'rtl';

  return (
    <AbsoluteFill style={{ background: '#070b16', color: '#f4f6fb', fontFamily: `${copy.font}, Vazirmatn, Inter, sans-serif`, overflow: 'hidden' }}>
      <AbsoluteFill style={{ background: 'radial-gradient(900px 480px at 88% -10%, rgba(91,124,255,0.28), transparent 60%), radial-gradient(700px 420px at 0% 110%, rgba(168,85,247,0.22), transparent 60%)' }} />
      <AbsoluteFill style={{
        backgroundImage: 'linear-gradient(rgba(148,163,184,0.07) 1px, transparent 1px), linear-gradient(90deg, rgba(148,163,184,0.07) 1px, transparent 1px)',
        backgroundSize: '56px 56px',
        maskImage: 'radial-gradient(ellipse 80% 70% at 50% 40%, #000 20%, transparent 75%)',
        WebkitMaskImage: 'radial-gradient(ellipse 80% 70% at 50% 40%, #000 20%, transparent 75%)',
      }} />

      <div style={{ position: 'absolute', top: 36, insetInline: 56, display: 'flex', alignItems: 'center', gap: 14, direction: copy.dir }}>
        <Logo />
        <span style={{ fontWeight: 800, fontSize: 26, letterSpacing: rtl ? 0 : -0.02 }}>{copy.brand}</span>
        <span style={{ marginInlineStart: 'auto', display: 'flex', gap: 8 }}>
          {copy.scenes.map((_, i) => (
            <i key={i} style={{ width: i === scene ? 28 : 8, height: 8, borderRadius: 99, background: i === scene ? '#8aa6ff' : 'rgba(148,163,184,0.35)' }} />
          ))}
        </span>
      </div>

      <div style={{
        position: 'absolute', top: 120, bottom: 48, insetInline: 72,
        display: 'flex', flexDirection: 'column', justifyContent: 'center', gap: 22,
        direction: copy.dir, textAlign: 'start', opacity,
      }}>
        <div style={{ ...rise(local, 0), color: '#a5b4fc', fontWeight: 700, fontSize: 22 }}>{data.kicker}</div>
        <h1 style={{ margin: 0, fontSize: rtl ? 64 : 68, lineHeight: rtl ? 1.25 : 1.05, fontWeight: 800, letterSpacing: rtl ? 0 : -1.5, whiteSpace: 'pre-line' }}>
          {data.title}
        </h1>
        <p style={{ margin: 0, maxWidth: 980, color: '#b7c3d9', fontSize: rtl ? 28 : 26, lineHeight: rtl ? 1.7 : 1.45, fontWeight: 500 }}>
          {data.body}
        </p>
        <div style={{ marginTop: 8, maxWidth: 980 }}>
          <Visual kind={data.visual} local={local} copy={copy} />
        </div>
      </div>
    </AbsoluteFill>
  );
};
