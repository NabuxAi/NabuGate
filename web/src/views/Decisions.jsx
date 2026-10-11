import { useEffect, useMemo, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { useT } from '../i18n/index.jsx';
import Icon from '../components/Icon.jsx';

/*
 * The decision-model playground: any chat alias the gateway routes to can be
 * driven as a decision model (one forward pass per question, letter options,
 * probabilities read off the option tokens), exactly the "build your own
 * decision model" recipe. Calibration fits the temperature that aligns the
 * engine's confidence with its measured accuracy.
 *
 * The console session cannot call /v1/* — only a gateway key can — so the view
 * creates (or accepts a pasted) playground key once and keeps it in
 * localStorage. It is a real key with a real balance: that is the point, the
 * calls are metered like any other project's.
 */

const T = {
  fa: {
    title: 'موتور تصمیم',
    subtitle: 'تصمیم‌های ساخت‌یافته با یک پاسِ رو به جلو — احتمال هر گزینه از logprob توکن‌های حرف گزینه‌ها',
    keyCard: 'کلید پلی‌گراند',
    keyExplain: 'این صفحه مستقیم به /v1/decisions تماس می‌زند؛ برای آن به یک کلید دروازه لازم است. یک کلید پلی‌گراند بسازید یا کلید موجودتان را بچسبانید. کلید فقط در مرورگر شما ذخیره می‌شود.',
    keyCreate: 'ساخت کلید پلی‌گراند',
    keyPaste: 'یا کلید موجود را بچسبانید:',
    keySave: 'ذخیره کلید',
    keyReady: 'کلید ذخیره شد',
    keyChange: 'تغییر کلید',
    playground: 'پلی‌گراند',
    model: 'مدل (alias)',
    engine: 'موتور',
    engineAuto: 'خودکار (بومی، بعد محلی)',
    engineLocal: 'محلی (logprob)',
    engineNative: 'بومی (Jev / alpha)',
    temperature: 'دمای کالیبراسیون (T)',
    tempHint: 'T=۱ خروجی خام مدل است. با کالیبراسیون یک T بزرگ‌تر (مثلاً ۳–۵) اطمینانِ بیش‌ازحد مدل را هم‌-scale با دقتِ واقعی می‌کند.',
    state: 'وضعیت (State)',
    stateHint: 'متنی یا JSON که زمینهٔ تصمیم را می‌سازد.',
    questions: 'پرسش‌ها',
    addQuestion: 'افزودن پرسش',
    qId: 'شناسه',
    qType: 'نوع',
    qInstructions: 'دستور پرسش',
    qOptions: 'گزینه‌ها (کلید — توضیح)',
    addOption: 'گزینه جدید',
    qLevels: 'سطح‌ها (با ویرگول جدا کنید، از کم به زیاد)',
    qTrueDesc: 'توضیح «بله»',
    qFalseDesc: 'توضیح «خیر»',
    run: 'اجرای تصمیم',
    running: 'در حال اجرا…',
    results: 'خروجی',
    confidence: 'اطمینان',
    probabilities: 'احتمال گزینه‌ها',
    usage: 'مصرف',
    provider: 'پرووایدر',
    calibration: 'کالیبراسیون',
    calibExplain: 'یک مجموعهٔ برچسب‌خورده بدهید (state + questions + پاسخ درست). دروازه همه را با T=۱ اجرا می‌کند، دمایی را برازش می‌دهد که NLL گزینهٔ درست را کمینه کند و نمودار reliability قبل و بعد را برمی‌گرداند.',
    samplesJson: 'نمونه‌ها (JSON)',
    fit: 'برازش دما',
    fitting: 'در حال برازش…',
    fittedT: 'دمای برازش‌شده',
    nll: 'NLL (قبل ← بعد)',
    accuracy: 'دقت',
    reliability: 'نمودار reliability',
    binsBefore: 'قبل از کالیبراسیون',
    binsAfter: 'بعد از کالیبراسیون',
    binConf: 'اطمینان',
    binAcc: 'دقت واقعی',
    applyT: 'اعمال این T روی پلی‌گراند',
    applied: 'اعمال شد',
    confRange: (v) => `${Math.round(v.lo * 100)}–${Math.round(v.hi * 100)}٪`,
    empty: 'هنوز اجرایی نبوده',
    error: 'خطا',
    qError: 'این پرسش جواب نگرفت',
    letters: 'به هر گزینه یک حرف (A، B، …) داده می‌شود؛ مدل باید فقط حرف را بگوید و احتمال‌ها از توکن‌های همان حروف خوانده می‌شود.',
  },
  en: {
    title: 'Decision engine',
    subtitle: 'Structured decisions in a single forward pass — per-option probabilities read off the option tokens',
    keyCard: 'Playground key',
    keyExplain: 'This page calls /v1/decisions directly, which needs a gateway key. Create a playground key once or paste an existing one. It is stored only in your browser.',
    keyCreate: 'Create playground key',
    keyPaste: 'Or paste an existing key:',
    keySave: 'Save key',
    keyReady: 'Key saved',
    keyChange: 'Change key',
    playground: 'Playground',
    model: 'Model (alias)',
    engine: 'Engine',
    engineAuto: 'Auto (native, then local)',
    engineLocal: 'Local (logprobs)',
    engineNative: 'Native (Jev / alpha)',
    temperature: 'Calibration temperature (T)',
    tempHint: 'T=1 is the raw model output. Calibration fits a larger T (typically 3–5) that rescales overconfident certainty down to measured accuracy.',
    state: 'State',
    stateHint: 'Text or JSON that sets the context of the decision.',
    questions: 'Questions',
    addQuestion: 'Add question',
    qId: 'ID',
    qType: 'Type',
    qInstructions: 'Instructions',
    qOptions: 'Options (key — description)',
    addOption: 'Add option',
    qLevels: 'Levels (comma-separated, low to high)',
    qTrueDesc: '“true” description',
    qFalseDesc: '“false” description',
    run: 'Run decision',
    running: 'Running…',
    results: 'Output',
    confidence: 'Confidence',
    probabilities: 'Option probabilities',
    usage: 'Usage',
    provider: 'Provider',
    calibration: 'Calibration',
    calibExplain: 'Give a labelled set (state + questions + the correct answer). The gateway runs it all at T=1, fits the temperature minimising the correct option’s NLL, and returns the reliability diagram before and after.',
    samplesJson: 'Samples (JSON)',
    fit: 'Fit temperature',
    fitting: 'Fitting…',
    fittedT: 'Fitted T',
    nll: 'NLL (before → after)',
    accuracy: 'Accuracy',
    reliability: 'Reliability diagram',
    binsBefore: 'Before calibration',
    binsAfter: 'After calibration',
    binConf: 'Confidence',
    binAcc: 'Accuracy',
    applyT: 'Apply this T to the playground',
    applied: 'Applied',
    confRange: (v) => `${Math.round(v.lo * 100)}–${Math.round(v.hi * 100)}%`,
    empty: 'Nothing run yet',
    error: 'Error',
    qError: 'This question failed',
    letters: 'Each option gets a letter (A, B, …); the model answers with the letter only and probabilities are read from those letter tokens.',
  },
};

const KEY_STORAGE = 'nabugate.decisions.key';
const TEMP_STORAGE = 'nabugate.decisions.temp';

const DEFAULT_STATE = 'The user asks: "What color is the sky on a clear day?"';

const DEFAULT_QUESTIONS = [
  {
    id: 'color',
    type: 'choice',
    instructions: 'Which color does the input refer to?',
    options: [
      { key: 'red', desc: 'the color of stop signs and strawberries' },
      { key: 'blue', desc: 'the color of a clear daytime sky' },
      { key: 'green', desc: 'the color of grass and leaves' },
    ],
    levels: 'Low, Medium, High',
    trueDesc: '',
    falseDesc: '',
  },
];

const DEFAULT_SAMPLES = JSON.stringify(
  [
    {
      state: 'Question: What color is the sky on a clear day?',
      questions: { color: { type: 'choice', instructions: 'Pick the color.', criteria: { red: 'stop-sign red', blue: 'clear-sky blue', green: 'grass green' } } },
      expected: { color: 'blue' },
    },
    {
      state: 'Question: Where would you most likely find a bat?',
      questions: { place: { type: 'choice', instructions: 'Pick the place.', criteria: { attic: 'a dark upper room of a house', cave: 'an underground chamber', zoo: 'an animal park' } } },
      expected: { place: 'attic' },
    },
    {
      state: 'Statement: Paris is the capital of France.',
      questions: { fact: { type: 'noul', instructions: 'Is the statement true?' } },
      expected: { fact: 'true' },
    },
    {
      state: 'Statement: The sun rises in the west.',
      questions: { fact: { type: 'noul', instructions: 'Is the statement true?' } },
      expected: { fact: 'false' },
    },
  ],
  null,
  2,
);

async function v1Call(path, key, body) {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + key },
    body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data?.error?.message || res.statusText);
  return data;
}

function toRequestQuestions(qs) {
  const out = {};
  for (const q of qs) {
    if (!q.id.trim()) continue;
    if (q.type === 'choice') {
      const crit = {};
      for (const o of q.options) if (o.key.trim()) crit[o.key.trim()] = o.desc;
      out[q.id.trim()] = { type: 'choice', instructions: q.instructions, criteria: crit };
    } else if (q.type === 'noul') {
      const crit = {};
      if (q.trueDesc) crit.true = q.trueDesc;
      if (q.falseDesc) crit.false = q.falseDesc;
      out[q.id.trim()] = { type: 'noul', instructions: q.instructions, criteria: crit };
    } else {
      const levels = q.levels.split(/[،,]/).map((s) => s.trim()).filter(Boolean);
      out[q.id.trim()] = { type: 'score', instructions: q.instructions, criteria: levels };
    }
  }
  return out;
}

function QuestionEditor({ q, set, t, remove }) {
  const patch = (p) => set({ ...q, ...p });
  return (
    <div className="dec-question">
      <div className="dec-q-head">
        <input className="input" value={q.id} onChange={(e) => patch({ id: e.target.value })} placeholder={t('qId')} />
        <select className="input" value={q.type} onChange={(e) => patch({ type: e.target.value })}>
          <option value="choice">choice</option>
          <option value="noul">noul</option>
          <option value="score">score</option>
        </select>
        <button type="button" className="btn btn-ghost" onClick={remove}>✕</button>
      </div>
      <input className="input" value={q.instructions} onChange={(e) => patch({ instructions: e.target.value })} placeholder={t('qInstructions')} />
      {q.type === 'choice' && (
        <div className="dec-options">
          {q.options.map((o, i) => (
            <div key={i} className="dec-option-row">
              <input className="input mono" value={o.key} onChange={(e) => { const options = [...q.options]; options[i] = { ...o, key: e.target.value }; patch({ options }); }} />
              <input className="input" value={o.desc} onChange={(e) => { const options = [...q.options]; options[i] = { ...o, desc: e.target.value }; patch({ options }); }} />
              <button type="button" className="btn btn-ghost" onClick={() => patch({ options: q.options.filter((_, j) => j !== i) })}>✕</button>
            </div>
          ))}
          <button type="button" className="btn btn-ghost" onClick={() => patch({ options: [...q.options, { key: '', desc: '' }] })}>+ {t('addOption')}</button>
        </div>
      )}
      {q.type === 'noul' && (
        <div className="dec-option-row">
          <input className="input" value={q.trueDesc} onChange={(e) => patch({ trueDesc: e.target.value })} placeholder={t('qTrueDesc')} />
          <input className="input" value={q.falseDesc} onChange={(e) => patch({ falseDesc: e.target.value })} placeholder={t('qFalseDesc')} />
        </div>
      )}
      {q.type === 'score' && (
        <input className="input" value={q.levels} onChange={(e) => patch({ levels: e.target.value })} placeholder={t('qLevels')} />
      )}
    </div>
  );
}

function Bars({ probabilities, chosen }) {
  const entries = Object.entries(probabilities).sort((a, b) => b[1] - a[1]);
  if (!entries.length) return null;
  return (
    <div className="dec-bars">
      {entries.map(([k, p]) => (
        <div key={k} className={'dec-bar-row' + (k === chosen ? ' chosen' : '')}>
          <span className="dec-bar-key mono">{k}</span>
          <span className="dec-bar-track"><span className="dec-bar-fill" style={{ width: Math.max(1, p * 100) + '%' }} /></span>
          <span className="dec-bar-val mono">{(p * 100).toFixed(1)}%</span>
        </div>
      ))}
    </div>
  );
}

function BinsChart({ bins, t }) {
  const rows = bins.filter((b) => b.count > 0);
  if (!rows.length) return <div className="card-sub">—</div>;
  return (
    <div className="dec-bins">
      <div className="dec-bin-row dec-bin-head">
        <span />
        <span>{t('binConf')}</span>
        <span>{t('binAcc')}</span>
      </div>
      {rows.map((b, i) => (
        <div key={i} className="dec-bin-row">
          <span className="mono">{t('confRange', b)}</span>
          <span className="dec-bin-track"><span className="dec-bin-fill conf" style={{ width: Math.max(1, b.avg_confidence * 100) + '%' }} /></span>
          <span className="dec-bin-track"><span className="dec-bin-fill acc" style={{ width: Math.max(1, b.accuracy * 100) + '%' }} /></span>
        </div>
      ))}
    </div>
  );
}

export default function Decisions() {
  const t = useT(T);

  const [apiKey, setApiKey] = useState(() => { try { return localStorage.getItem(KEY_STORAGE) || ''; } catch { return ''; } });
  const [pastedKey, setPastedKey] = useState('');
  const [keyBusy, setKeyBusy] = useState(false);
  const [keyError, setKeyError] = useState(null);

  const [aliases, setAliases] = useState([]);
  const [model, setModel] = useState('');
  const [engine, setEngine] = useState('local');
  const [temperature, setTemperature] = useState(() => { const v = Number(localStorage.getItem(TEMP_STORAGE)); return v > 0 ? v : 1; });

  const [state, setState] = useState(DEFAULT_STATE);
  const [questions, setQuestions] = useState(DEFAULT_QUESTIONS);
  const [result, setResult] = useState(null);
  const [runError, setRunError] = useState(null);
  const [busy, setBusy] = useState(false);

  const [samples, setSamples] = useState(DEFAULT_SAMPLES);
  const [calib, setCalib] = useState(null);
  const [calibError, setCalibError] = useState(null);
  const [calibBusy, setCalibBusy] = useState(false);

  useEffect(() => {
    api.aliases()
      .then((r) => {
        const usable = (r.aliases || []).filter((a) => a.kind === 'chat' || a.kind === 'decision');
        setAliases(usable);
        if (usable.length && !usable.some((a) => a.alias === model)) {
          setModel(usable.find((a) => a.alias === 'nabu-fast')?.alias || usable[0].alias);
        }
      })
      .catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => { try { localStorage.setItem(TEMP_STORAGE, String(temperature)); } catch { /* private mode */ } }, [temperature]);
  useEffect(() => {
    try { apiKey ? localStorage.setItem(KEY_STORAGE, apiKey) : localStorage.removeItem(KEY_STORAGE); } catch { /* private mode */ }
  }, [apiKey]);

  async function createPlaygroundKey() {
    setKeyBusy(true);
    setKeyError(null);
    try {
      const r = await api.createToken({ name: 'decisions-playground', allow: [], rateLimit: 60 });
      setApiKey(r.secret);
    } catch (err) {
      setKeyError(err.message);
    } finally {
      setKeyBusy(false);
    }
  }

  async function run() {
    setBusy(true);
    setRunError(null);
    setResult(null);
    try {
      const body = { model, state, questions: toRequestQuestions(questions), engine };
      if (Number(temperature) !== 1) body.temperature = Number(temperature);
      const r = await v1Call('/v1/decisions', apiKey, body);
      setResult(r);
    } catch (err) {
      setRunError(err.message);
    } finally {
      setBusy(false);
    }
  }

  async function calibrate() {
    setCalibBusy(true);
    setCalibError(null);
    setCalib(null);
    try {
      const parsed = JSON.parse(samples);
      const r = await v1Call('/v1/decisions/calibrate', apiKey, { model, engine, samples: parsed });
      setCalib(r);
    } catch (err) {
      setCalibError(err.message);
    } finally {
      setCalibBusy(false);
    }
  }

  const resultQuestions = useMemo(() => (result ? Object.entries(result.answers || {}) : []), [result]);

  return (
    <Layout title={t('title')} subtitle={t('subtitle')}>
      <div className="grid grid-2-wide">
        <div className="col">
          <section className="card">
            <h3>{t('keyCard')}</h3>
            {apiKey ? (
              <div className="dec-key-row">
                <span className="badge badge-ok">{t('keyReady')}</span>
                <span className="mono">{apiKey.slice(0, 8)}…{apiKey.slice(-4)}</span>
                <button type="button" className="btn btn-ghost" onClick={() => setApiKey('')}>{t('keyChange')}</button>
              </div>
            ) : (
              <>
                <p className="card-sub">{t('keyExplain')}</p>
                <div className="dec-key-row">
                  <button type="button" className="btn btn-primary" onClick={createPlaygroundKey} disabled={keyBusy}>{t('keyCreate')}</button>
                  <span className="input-wrap">
                    <input className="input mono" value={pastedKey} onChange={(e) => setPastedKey(e.target.value)} placeholder="nabu-…" />
                  </span>
                  <button type="button" className="btn btn-ghost" onClick={() => setPastedKey.trim() && setApiKey(pastedKey.trim())} disabled={!pastedKey.trim()}>{t('keySave')}</button>
                </div>
                {keyError && <p className="dec-error">{t('error')}: {keyError}</p>}
              </>
            )}
          </section>

          <section className="card">
            <h3>{t('playground')}</h3>
            <p className="card-sub">{t('letters')}</p>
            <div className="dec-controls">
              <label>
                <span>{t('model')}</span>
                <select className="input" value={model} onChange={(e) => setModel(e.target.value)}>
                  {aliases.map((a) => <option key={a.alias} value={a.alias}>{a.alias} · {a.kind}</option>)}
                </select>
              </label>
              <label>
                <span>{t('engine')}</span>
                <select className="input" value={engine} onChange={(e) => setEngine(e.target.value)}>
                  <option value="auto">{t('engineAuto')}</option>
                  <option value="local">{t('engineLocal')}</option>
                  <option value="native">{t('engineNative')}</option>
                </select>
              </label>
              <label className="dec-temp">
                <span>{t('temperature')}: <b className="mono">{Number(temperature).toFixed(1)}</b></span>
                <input type="range" min="0.5" max="10" step="0.1" value={temperature} onChange={(e) => setTemperature(e.target.value)} />
                <small>{t('tempHint')}</small>
              </label>
            </div>
            <label className="dec-state">
              <span>{t('state')} <small>{t('stateHint')}</small></span>
              <textarea className="input" rows="3" value={state} onChange={(e) => setState(e.target.value)} />
            </label>
            <div className="dec-questions">
              <h4>{t('questions')}</h4>
              {questions.map((q, i) => (
                <QuestionEditor
                  key={i}
                  q={q}
                  t={t}
                  set={(nq) => setQuestions(questions.map((x, j) => (j === i ? nq : x)))}
                  remove={() => setQuestions(questions.filter((_, j) => j !== i))}
                />
              ))}
              <button
                type="button"
                className="btn btn-ghost"
                onClick={() => setQuestions([...questions, { id: '', type: 'choice', instructions: '', options: [{ key: '', desc: '' }], levels: 'Low, Medium, High', trueDesc: '', falseDesc: '' }])}
              >+ {t('addQuestion')}</button>
            </div>
            <div className="dec-run-row">
              <button type="button" className="btn btn-primary" onClick={run} disabled={busy || !apiKey || !model || !questions.some((q) => q.id.trim())}>
                {busy ? t('running') : t('run')}
              </button>
            </div>

            {runError && <p className="dec-error">{t('error')}: {runError}</p>}
            {result && (
              <div className="dec-result">
                <div className="dec-meta">
                  <span className="tag tag-primary">{result.engine}</span>
                  <span className="tag">{result.model}</span>
                  <span className="tag">{t('usage')}: {result.usage?.total_tokens ?? 0}</span>
                </div>
                {resultQuestions.map(([qid, a]) => (
                  <div key={qid} className="dec-answer">
                    <div className="dec-answer-head">
                      <b className="mono">{qid}</b>
                      <span className="badge badge-info">{a.type}</span>
                      {a.error
                        ? <span className="badge badge-warn">{t('qError')}</span>
                        : <span className="dec-answer-pick">
                            {a.type === 'choice' && <b className="mono">{a.choice}</b>}
                            {a.type === 'noul' && <b>{a.noul >= 0.5 ? 'true' : 'false'} ({(a.noul * 100).toFixed(1)}%)</b>}
                            {a.type === 'score' && <b>{Number(a.score).toFixed(2)} · {a.legend?.[String(Math.round(a.score))]}</b>}
                            <span className="dec-conf">{t('confidence')}: <b className="mono">{(a.confidence * 100).toFixed(1)}%</b></span>
                          </span>}
                    </div>
                    {a.error ? <p className="dec-error mono">{a.error}</p> : <Bars probabilities={a.probabilities || {}} chosen={a.choice} />}
                  </div>
                ))}
              </div>
            )}
            {!result && !runError && <p className="card-sub">{t('empty')}</p>}
          </section>
        </div>

        <div className="col">
          <section className="card">
            <h3>{t('calibration')}</h3>
            <p className="card-sub">{t('calibExplain')}</p>
            <label className="dec-state">
              <span>{t('samplesJson')}</span>
              <textarea className="input mono" rows="14" value={samples} onChange={(e) => setSamples(e.target.value)} />
            </label>
            <button type="button" className="btn btn-primary" onClick={calibrate} disabled={calibBusy || !apiKey || !model}>
              {calibBusy ? t('fitting') : t('fit')}
            </button>
            {calibError && <p className="dec-error">{t('error')}: {calibError}</p>}
            {calib && (
              <div className="dec-calib">
                <div className="stat-grid dec-stat-grid">
                  <div className="stat"><div className="label">{t('fittedT')}</div><div className="value">{Number(calib.temperature).toFixed(2)}</div></div>
                  <div className="stat"><div className="label">{t('accuracy')}</div><div className="value">{(calib.accuracy * 100).toFixed(1)}%</div></div>
                  <div className="stat"><div className="label">{t('nll')}</div><div className="value">{Number(calib.nll_before).toFixed(3)} → {Number(calib.nll_after).toFixed(3)}</div></div>
                </div>
                <div className="dec-bins-block">
                  <h4>{t('binsBefore')}</h4>
                  <BinsChart bins={calib.bins_before || []} t={t} />
                  <h4>{t('binsAfter')}</h4>
                  <BinsChart bins={calib.bins_after || []} t={t} />
                </div>
                <button type="button" className="btn" onClick={() => setTemperature(Number(calib.temperature))}>{t('applyT')} ({Number(calib.temperature).toFixed(2)})</button>
              </div>
            )}
          </section>
        </div>
      </div>
    </Layout>
  );
}
