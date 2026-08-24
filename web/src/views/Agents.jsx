import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';

export default function Agents() {
  const [tab, setTab] = useState('agents'); // 'agents' | 'flows' | 'playground' | 'builder'
  const [agents, setAgents] = useState([]);
  const [flows, setFlows] = useState([]);
  const [search, setSearch] = useState('');
  const [squadFilter, setSquadFilter] = useState('all');
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);

  // Edit / Create Modal State
  const [editingAgent, setEditingAgent] = useState(null);
  const [editingFlow, setEditingFlow] = useState(null);

  // Playground State
  const [testTarget, setTestTarget] = useState('seo-audit-team');
  const [testType, setTestType] = useState('flow'); // 'agent' | 'flow'
  const [testPrompt, setTestPrompt] = useState('');
  const [testRunning, setTestRunning] = useState(false);
  const [testResponse, setTestResponse] = useState(null);

  const loadData = () => {
    api.listAgents()
      .then((r) => setAgents(r.agents || []))
      .catch((e) => setError(e.message));

    api.listFlows()
      .then((r) => setFlows(r.flows || []))
      .catch(() => {});
  };

  useEffect(loadData, []);

  // Save Agent
  async function handleSaveAgent(e) {
    e.preventDefault();
    try {
      await api.saveAgent(editingAgent);
      setEditingAgent(null);
      setSuccess(`ساب‌اجنت «${editingAgent.name}» با موفقیت ذخیره شد.`);
      loadData();
      setTimeout(() => setSuccess(null), 3000);
    } catch (err) {
      setError(err.message);
    }
  }

  // Delete Agent
  async function handleDeleteAgent(name) {
    if (!confirm(`آیا از حذف ساب‌اجنت «${name}» اطمینان دارید؟`)) return;
    try {
      await api.deleteAgent(name);
      setSuccess(`ساب‌اجنت «${name}» حذف شد.`);
      loadData();
      setTimeout(() => setSuccess(null), 3000);
    } catch (err) {
      setError(err.message);
    }
  }

  // Save Flow
  async function handleSaveFlow(e) {
    e.preventDefault();
    try {
      await api.saveFlow(editingFlow);
      setEditingFlow(null);
      setSuccess(`خط‌لوله «${editingFlow.name}» با موفقیت ذخیره شد.`);
      loadData();
      setTimeout(() => setSuccess(null), 3000);
    } catch (err) {
      setError(err.message);
    }
  }

  // Delete Flow
  async function handleDeleteFlow(name) {
    if (!confirm(`آیا از حذف خط‌لوله «${name}» اطمینان دارید؟`)) return;
    try {
      await api.deleteFlow(name);
      setSuccess(`خط‌لوله «${name}» حذف شد.`);
      loadData();
      setTimeout(() => setSuccess(null), 3000);
    } catch (err) {
      setError(err.message);
    }
  }

  // Run Test in Playground
  async function handleRunTest(e) {
    e.preventDefault();
    if (!testPrompt.trim()) return;
    setTestRunning(true);
    setTestResponse(null);
    setError(null);
    try {
      let res;
      if (testType === 'flow') {
        res = await api.testFlow(testTarget, testPrompt);
      } else {
        res = await api.testAgent(testTarget, testPrompt);
      }
      setTestResponse(res?.response || res?.content || JSON.stringify(res, null, 2));
    } catch (err) {
      setError(err.message);
    } finally {
      setTestRunning(false);
    }
  }

  // Filter Agents
  const filteredAgents = agents.filter((a) => {
    const matchesSearch =
      a.name.toLowerCase().includes(search.toLowerCase()) ||
      (a.description && a.description.toLowerCase().includes(search.toLowerCase()));

    if (!matchesSearch) return false;
    if (squadFilter === 'all') return true;
    if (squadFilter === 'seo') return a.name.startsWith('seo-');
    if (squadFilter === 'cine') return a.name.startsWith('cine-');
    if (squadFilter === 'sales') return a.name.startsWith('sales-');
    if (squadFilter === 'write') return a.name.startsWith('write-');
    return true;
  });

  return (
    <Layout
      title="استودیو ایجنت‌ها و فلوها"
      subtitle="مدیریت، ویرایش زنده، ساخت تیم‌های چند ایجنتی و تست تعاملی"
      actions={
        <div style={{ display: 'flex', gap: '8px' }}>
          <button
            className="btn btn-secondary"
            onClick={() =>
              setEditingFlow({
                name: 'new-flow',
                description: 'توضیح خط‌لوله',
                steps: [
                  { agent: 'seo-content-auditor', label: 'تحلیل اولیه' },
                  { agent: 'seo-strategist-reviewer', label: 'بازبینی' },
                ],
              })
            }
          >
            + ساخت خط‌لوله (Flow)
          </button>
          <button
            className="btn btn-primary"
            onClick={() =>
              setEditingAgent({
                name: 'new-agent',
                description: '',
                model: 'nabu-smart',
                temperature: 0.3,
                max_tokens: 4096,
                system: 'دستورات سیستمی ایجنت را اینجا وارد کنید.',
              })
            }
          >
            + ساخت ساب‌اجنت
          </button>
        </div>
      }
    >
      {error && <div className="banner-error" style={{ marginBottom: '16px' }}>{error}</div>}
      {success && <div className="banner-success" style={{ marginBottom: '16px', background: '#052e16', border: '1px solid #16a34a', color: '#86efac', padding: '12px 16px', borderRadius: '8px' }}>{success}</div>}

      <div className="tabs-container">
        <div
          className={`tab-item ${tab === 'agents' ? 'active' : ''}`}
          onClick={() => setTab('agents')}
        >
          <span>🤖</span> ساب‌اجنت‌ها ({agents.length})
        </div>
        <div
          className={`tab-item ${tab === 'flows' ? 'active' : ''}`}
          onClick={() => setTab('flows')}
        >
          <span>🔄</span> خط‌لوله‌ها / Flows ({flows.length})
        </div>
        <div
          className={`tab-item ${tab === 'playground' ? 'active' : ''}`}
          onClick={() => setTab('playground')}
        >
          <span>⚡</span> پلی‌گراند و تست زنده
        </div>
      </div>

      {/* TAB 1: AGENTS */}
      {tab === 'agents' && (
        <>
          <div style={{ display: 'flex', gap: '12px', marginBottom: '24px', flexWrap: 'wrap', alignItems: 'center' }}>
            <input
              type="text"
              placeholder="جستجو در ساب‌اجنت‌ها..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="input"
              style={{ flex: 1, minWidth: '240px', maxWidth: '300px' }}
            />
            <div style={{ display: 'flex', gap: '6px' }}>
              {['all', 'seo', 'cine', 'sales', 'write'].map((sq) => (
                <button
                  key={sq}
                  className={`btn btn-sm ${squadFilter === sq ? 'btn-primary' : 'btn-secondary'}`}
                  style={{ borderRadius: '20px', padding: '6px 14px' }}
                  onClick={() => setSquadFilter(sq)}
                >
                  {sq === 'all' ? 'همه' : sq.toUpperCase()}
                </button>
              ))}
            </div>
          </div>

          <div className="grid grid-3">
            {filteredAgents.map((a) => (
              <div key={a.name} className="card" style={{ display: 'flex', flexDirection: 'column', justifyContent: 'space-between', transition: 'transform 0.2s', cursor: 'pointer' }} onMouseEnter={e => e.currentTarget.style.transform = 'translateY(-2px)'} onMouseLeave={e => e.currentTarget.style.transform = 'translateY(0)'}>
                <div>
                  <div className="card-head">
                    <span className="mono" style={{ fontWeight: '800', color: 'var(--ng-heading)', fontSize: '14px' }}>{a.name}</span>
                    <span className="pill pill-plain">{a.model || 'nabu-smart'}</span>
                  </div>
                  <p className="card-sub" style={{ minHeight: '40px', lineHeight: '1.5' }}>
                    {a.description || 'بدون توضیح'}
                  </p>
                  {a.system && (
                    <div style={{ background: 'var(--ng-surface-soft)', border: '1px solid var(--ng-border-faint)', padding: '10px 14px', borderRadius: '8px', fontSize: '12px', color: 'var(--ng-muted)', maxHeight: '70px', overflow: 'hidden', margin: '12px 0', lineHeight: '1.6' }}>
                      {a.system}
                    </div>
                  )}
                </div>
                <div style={{ display: 'flex', gap: '8px', marginTop: '16px', borderTop: '1px solid var(--ng-border-faint)', paddingTop: '16px' }}>
                  <button
                    className="btn btn-ghost"
                    style={{ flex: 1, justifyContent: 'center' }}
                    onClick={() => setEditingAgent({ ...a })}
                  >
                    ✏️ ویرایش
                  </button>
                  <button
                    className="btn btn-ghost"
                    style={{ flex: 1, justifyContent: 'center' }}
                    onClick={() => {
                      setTestTarget(a.name);
                      setTestType('agent');
                      setTab('playground');
                    }}
                  >
                    ⚡ تست
                  </button>
                  <button
                    className="btn btn-ghost"
                    style={{ color: '#ef4444', borderColor: 'transparent' }}
                    onClick={() => handleDeleteAgent(a.name)}
                    title="حذف"
                  >
                    🗑️
                  </button>
                </div>
              </div>
            ))}
          </div>
        </>
      )}

      {/* TAB 2: FLOWS */}
      {tab === 'flows' && (
        <div className="grid grid-2-wide">
          {flows.map((f) => (
            <div key={f.name} className="card" style={{ display: 'flex', flexDirection: 'column', justifyContent: 'space-between' }}>
              <div>
                <div className="card-head">
                  <span className="mono" style={{ fontWeight: '800', color: 'var(--ng-heading)', fontSize: '14px' }}>{f.name}</span>
                </div>
                <p className="card-sub" style={{ lineHeight: '1.5' }}>
                  {f.description || 'بدون توضیح'}
                </p>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginTop: '16px', background: 'var(--ng-surface-soft)', padding: '12px', borderRadius: '8px' }}>
                  <strong style={{ fontSize: '11.5px', color: 'var(--ng-slate-700)' }}>مراحل (Steps):</strong>
                  {f.steps?.map((st, i) => (
                    <div key={i} style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '12.5px' }}>
                      <span style={{ width: '20px', height: '20px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--ng-accent)', color: '#fff', borderRadius: '50%', fontSize: '11px', fontWeight: 'bold' }}>{i + 1}</span>
                      <span className="mono" style={{ color: 'var(--ng-heading)', fontWeight: '600' }}>{st.agent}</span>
                      {st.label && <span style={{ color: 'var(--ng-muted)', fontSize: '11px' }}>({st.label})</span>}
                    </div>
                  ))}
                </div>
              </div>
              <div style={{ display: 'flex', gap: '8px', marginTop: '16px', borderTop: '1px solid var(--ng-border-faint)', paddingTop: '16px' }}>
                <button
                  className="btn btn-ghost"
                  style={{ flex: 1, justifyContent: 'center' }}
                  onClick={() => setEditingFlow({ ...f })}
                >
                  ✏️ ویرایش
                </button>
                <button
                  className="btn btn-ghost"
                  style={{ flex: 1, justifyContent: 'center' }}
                  onClick={() => {
                    setTestTarget(f.name);
                    setTestType('flow');
                    setTab('playground');
                  }}
                >
                  ⚡ اجرای فلو
                </button>
                <button
                  className="btn btn-ghost"
                  style={{ color: '#ef4444', borderColor: 'transparent' }}
                  onClick={() => handleDeleteFlow(f.name)}
                >
                  🗑️
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* TAB 3: PLAYGROUND */}
      {tab === 'playground' && (
        <div className="playground-card">
          <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginBottom: '24px' }}>
            <div style={{ background: 'var(--ng-accent)', color: 'white', padding: '10px', borderRadius: '12px', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              ⚡
            </div>
            <div>
              <h3 style={{ margin: 0, fontSize: '18px', color: 'var(--ng-heading)' }}>پلی‌گراند اجرای زنده</h3>
              <p style={{ margin: '4px 0 0 0', fontSize: '12.5px', color: 'var(--ng-muted)' }}>اجرای سریع ایجنت‌ها و خط‌لوله‌ها برای تست پرامپت و خروجی.</p>
            </div>
          </div>

          <form onSubmit={handleRunTest}>
            <div style={{ display: 'flex', gap: '16px', marginBottom: '20px' }}>
              <div style={{ flex: 1 }}>
                <label className="label">نوع هدف (Target Type)</label>
                <select
                  className="input"
                  value={testType}
                  onChange={(e) => {
                    setTestType(e.target.value);
                    setTestTarget(e.target.value === 'flow' ? (flows[0]?.name || 'seo-audit-team') : (agents[0]?.name || 'seo-content-auditor'));
                  }}
                >
                  <option value="flow">خط‌لوله (Flow)</option>
                  <option value="agent">ساب‌اجنت تکی (Sub-Agent)</option>
                </select>
              </div>
              <div style={{ flex: 2 }}>
                <label className="label">نام ایجنت / خط‌لوله</label>
                <select
                  className="input mono"
                  value={testTarget}
                  onChange={(e) => setTestTarget(e.target.value)}
                >
                  {testType === 'flow'
                    ? flows.map((f) => <option key={f.name} value={f.name}>{f.name}</option>)
                    : agents.map((a) => <option key={a.name} value={a.name}>{a.name}</option>)}
                </select>
              </div>
            </div>

            <div style={{ marginBottom: '24px' }}>
              <label className="label">ورودی / متن مقاله / پرامپت (Input)</label>
              <textarea
                className="input"
                rows={6}
                placeholder="متن خود را برای ارسال به ایجنت یا فلو وارد کنید..."
                value={testPrompt}
                onChange={(e) => setTestPrompt(e.target.value)}
                required
              />
            </div>

            <button type="submit" className="btn btn-primary animated-btn" disabled={testRunning} style={{ width: '100%', fontSize: '14px', padding: '14px' }}>
              {testRunning ? '⏳ در حال اجرا و پردازش...' : '🚀 ارسال و اجرای آنی'}
            </button>
          </form>

          {testResponse && (
            <div style={{ marginTop: '32px', borderTop: '1px solid var(--ng-border-faint)', paddingTop: '24px', animation: 'fadeIn 0.4s ease-out' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
                <h4 style={{ margin: 0, color: 'var(--ng-heading)', fontSize: '15px', display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ color: 'var(--ng-ok-text)' }}>●</span> پاسخ دریافتی:
                </h4>
                <button
                  className="btn btn-ghost"
                  onClick={() => navigator.clipboard.writeText(testResponse)}
                >
                  📋 کپی متن
                </button>
              </div>
              <pre className="response-box">
                {testResponse}
              </pre>
            </div>
          )}
        </div>
      )}

      {/* EDIT AGENT MODAL */}
      {editingAgent && (
        <div className="modal-backdrop">
          <div className="card modal glass-card" style={{ maxWidth: '680px' }}>
            <h3 style={{ borderBottom: '1px solid var(--ng-border-faint)', paddingBottom: '16px', marginBottom: '20px' }}>
              {editingAgent.name ? `ویرایش ساب‌اجنت ${editingAgent.name}` : 'ساخت ساب‌اجنت جدید'}
            </h3>
            <form onSubmit={handleSaveAgent}>
              <div style={{ marginBottom: '16px' }}>
                <label className="label">نام ایجنت (Slug)</label>
                <input
                  type="text"
                  className="input mono"
                  value={editingAgent.name}
                  onChange={(e) => setEditingAgent({ ...editingAgent, name: e.target.value })}
                  required
                />
              </div>
              <div style={{ marginBottom: '16px' }}>
                <label className="label">توضیحات</label>
                <input
                  type="text"
                  className="input"
                  value={editingAgent.description || ''}
                  onChange={(e) => setEditingAgent({ ...editingAgent, description: e.target.value })}
                />
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1.5fr 1fr', gap: '16px', marginBottom: '20px' }}>
                <div>
                  <label className="label">مدل پایه (Model Alias)</label>
                  <select
                    className="input mono"
                    value={editingAgent.model || 'nabu-smart'}
                    onChange={(e) => setEditingAgent({ ...editingAgent, model: e.target.value })}
                  >
                    <option value="nabu-smart">nabu-smart (Smart: Gemini 2.5 Pro / GPT-4o)</option>
                    <option value="nabu-fast">nabu-fast (Fast: Gemini 2.5 Flash / Groq)</option>
                    <option value="nabu-cheap">nabu-cheap (Cheap: Flash-Lite / Qwen)</option>
                  </select>
                </div>
                <div>
                  <label className="label">Temperature (دقت/خلاقیت)</label>
                  <input
                    type="number"
                    step="0.1"
                    min="0"
                    max="2"
                    className="input"
                    value={editingAgent.temperature ?? 0.3}
                    onChange={(e) => setEditingAgent({ ...editingAgent, temperature: parseFloat(e.target.value) })}
                  />
                </div>
              </div>
              <div style={{ marginBottom: '16px' }}>
                <label className="label">دستورات سیستمی (System Prompt)</label>
                <textarea
                  className="input"
                  rows={8}
                  value={editingAgent.system || ''}
                  onChange={(e) => setEditingAgent({ ...editingAgent, system: e.target.value })}
                  required
                />
              </div>
              <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
                <button type="button" className="btn btn-secondary" onClick={() => setEditingAgent(null)}>
                  انصراف
                </button>
                <button type="submit" className="btn btn-primary">
                  💾 ذخیره ساب‌اجنت
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* EDIT FLOW MODAL */}
      {editingFlow && (
        <div className="modal-backdrop">
          <div className="card modal glass-card" style={{ maxWidth: '680px' }}>
            <h3 style={{ borderBottom: '1px solid var(--ng-border-faint)', paddingBottom: '16px', marginBottom: '20px' }}>
              {editingFlow.name ? `ویرایش خط‌لوله ${editingFlow.name}` : 'ساخت خط‌لوله جدید'}
            </h3>
            <form onSubmit={handleSaveFlow}>
              <div style={{ marginBottom: '16px' }}>
                <label className="label">نام خط‌لوله (Flow Name)</label>
                <input
                  type="text"
                  className="input mono"
                  value={editingFlow.name}
                  onChange={(e) => setEditingFlow({ ...editingFlow, name: e.target.value })}
                  required
                />
              </div>
              <div style={{ marginBottom: '16px' }}>
                <label className="label">توضیحات</label>
                <input
                  type="text"
                  className="input"
                  value={editingFlow.description || ''}
                  onChange={(e) => setEditingFlow({ ...editingFlow, description: e.target.value })}
                />
              </div>

              <div style={{ marginBottom: '24px' }}>
                <label className="label" style={{ marginBottom: '12px' }}>گام‌های خط‌لوله (Steps)</label>
                <div style={{ background: 'var(--ng-surface-soft)', padding: '16px', borderRadius: '12px', border: '1px solid var(--ng-border-faint)' }}>
                  {editingFlow.steps?.map((st, i) => (
                    <div key={i} style={{ display: 'flex', gap: '12px', alignItems: 'center', marginBottom: '12px' }}>
                      <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: '24px', height: '24px', background: 'var(--ng-surface)', borderRadius: '50%', color: 'var(--ng-muted)', fontSize: '11px', fontWeight: 'bold', border: '1px solid var(--ng-border-faint)' }}>
                        {i + 1}
                      </span>
                      <select
                        className="input mono"
                        style={{ flex: 2, padding: '8px 12px' }}
                        value={st.agent}
                        onChange={(e) => {
                          const newSteps = [...editingFlow.steps];
                          newSteps[i].agent = e.target.value;
                          setEditingFlow({ ...editingFlow, steps: newSteps });
                        }}
                      >
                        {agents.map((a) => (
                          <option key={a.name} value={a.name}>{a.name}</option>
                        ))}
                      </select>
                      <input
                        type="text"
                        placeholder="عنوان گام"
                        className="input"
                        style={{ flex: 2, padding: '8px 12px' }}
                        value={st.label || ''}
                        onChange={(e) => {
                          const newSteps = [...editingFlow.steps];
                          newSteps[i].label = e.target.value;
                          setEditingFlow({ ...editingFlow, steps: newSteps });
                        }}
                      />
                      <button
                        type="button"
                        className="btn btn-ghost"
                        style={{ padding: '8px', color: '#ef4444' }}
                        onClick={() => {
                          const newSteps = editingFlow.steps.filter((_, idx) => idx !== i);
                          setEditingFlow({ ...editingFlow, steps: newSteps });
                        }}
                      >
                        ✕
                      </button>
                    </div>
                  ))}
                  <button
                    type="button"
                    className="btn btn-ghost"
                    style={{ width: '100%', border: '1px dashed var(--ng-border-faint)', color: 'var(--ng-heading)' }}
                    onClick={() =>
                      setEditingFlow({
                        ...editingFlow,
                        steps: [...(editingFlow.steps || []), { agent: agents[0]?.name || 'seo-content-auditor', label: `گام ${(editingFlow.steps?.length || 0) + 1}` }],
                      })
                    }
                  >
                    + افزودن گام جدید
                  </button>
                </div>
              </div>

              <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
                <button type="button" className="btn btn-secondary" onClick={() => setEditingFlow(null)}>
                  انصراف
                </button>
                <button type="submit" className="btn btn-primary">
                  💾 ذخیره خط‌لوله
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </Layout>
  );
}
