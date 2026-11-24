import React from 'react';
import { Composition } from 'remotion';
import { Intro } from './Intro.jsx';
import { COPY, DURATION, FPS, LANGS } from './copy.js';
import { Guide, guideDuration } from './Guide.jsx';
import { GUIDE_FPS, GUIDE_LANGS } from './guideCopy.js';

export const RemotionRoot = () => (
  <>
    {LANGS.map((lang) => (
      <Composition
        key={lang}
        id={`intro-${lang}`}
        component={Intro}
        durationInFrames={DURATION}
        fps={FPS}
        width={1280}
        height={720}
        defaultProps={{ lang }}
      />
    ))}
    {GUIDE_LANGS.map((lang) => (
      <Composition
        key={`guide-${lang}`}
        id={`guide-${lang}`}
        component={Guide}
        durationInFrames={guideDuration(lang)}
        fps={GUIDE_FPS}
        width={1280}
        height={720}
        defaultProps={{ lang }}
      />
    ))}
  </>
);

export const compositionMeta = { COPY, DURATION, FPS };
