import React from 'react';
import { Composition } from 'remotion';
import { Intro } from './Intro.jsx';
import { COPY, DURATION, FPS, LANGS } from './copy.js';

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
  </>
);

export const compositionMeta = { COPY, DURATION, FPS };
