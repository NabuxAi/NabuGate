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

      {/* Tabs */}
      <div style={{ display: 'flex', gap: '8px', borderBottom: '1px solid var(--border)', paddingBottom: '12px', marginBottom: '24px' }}>
        <button
          className={`btn ${tab === 'agents' ? 'btn-primary' : 'btn-secondary'}`}
          onClick={() => setTab('agents')}
        >
          🤖 ساب‌اجنت‌ها ({agents.length})
        </button>
        <button
          className={`btn ${tab === 'flows' ? 'btn-primary' : 'btn-secondary'}`}
          onClick={() => setTab('flows')}
        >
          🔄 خط‌لوله‌ها / Flows ({flows.length})
        </button>
        <button
          className={`btn ${tab === 'playground' ? 'btn-primary' : 'btn-secondary'}`}
          onClick={() => setTab('playground')}
        >
          ⚡ پلی‌گراند و تست زنده
        </button>
      </div>

      {/* TAB 1: AGENTS */}
      {tab === 'agents' && (
        <>
          <div style={{ display: 'flex', gap: '12px', marginBottom: '20px', flexWrap: 'wrap', alignItems: 'center' }}>
            <input
              type="text"
              placeholder="جستجو در ساب‌اجنت‌ها..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="input"
              style={{ flex: 1, minWidth: '240px' }}
            />
            <div style={{ display: 'flex', gap: '6px' }}>
              {['all', 'seo', 'cine', 'sales', 'write'].map((sq) => (
                <button
                  key={sq}
                  className={`btn btn-sm ${squadFilter === sq ? 'btn-primary' : 'btn-secondary'}`}
                  onClick={() => setSquadFilter(sq)}
                >
                  {sq === 'all' ? 'همه' : sq.toUpperCase()}
                </button>
              ))}
            </div>
          </div>

          <div className="grid grid-3">
            {filteredAgents.map((a) => (
              <div key={a.name} className="card" style={{ display: 'flex', flexDirection: 'column', justifyContent: 'space-between' }}>
                <div>
                  <div className="card-head" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <span className="mono" style={{ fontWeight: 'bold', color: 'var(--accent)' }}>{a.name}</span>
                    <span className="badge" style={{ fontSize: '0.75rem' }}>{a.model || 'nabu-smart'}</span>
                  </div>
                  <p className="card-sub" style={{ marginTop: '8px', minHeight: '40px' }}>
                    {a.description || 'بدون توضیح'}
                  </p>
                  {a.system && (
                    <div style={{ background: 'var(--bg-subtle, #0b1329)', padding: '8px 12px', borderRadius: '6px', fontSize: '0.8rem', color: '#94a3b8', maxHeight: '70px', overflow: 'hidden', margin: '8px 0' }}>
                      {a.system}
                    </div>
                  )}
                </div>
                <div style={{ display: 'flex', gap: '6px', marginTop: '16px', borderTop: '1px solid var(--border)', paddingTop: '12px' }}>
                  <button
                    className="btn btn-sm btn-secondary"
                    style={{ flex: 1 }}
                    onClick={() => setEditingAgent({ ...a })}
                  >
                    ✏️ ویرایش
                  </button>
                  <button
                    className="btn btn-sm btn-secondary"
                    onClick={() => {
                      setTestTarget(a.name);
                      setTestType('agent');
                      setTab('playground');
                    }}
                  >
                    ⚡ تست
                  </button>
                  <button
                    className="btn btn-sm btn-danger"
                    onClick={() => handleDeleteAgent(a.name)}
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
        <div className="grid grid-2">
          {flows.map((f) => (
            <div key={f.name} className="card">
              <div className="card-head" style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span className="mono" style={{ fontWeight: 'bold', color: '#38bdf8' }}>{f.name}</span>
                <span className="badge">{f.steps?.length || 0} گام</span>
              </div>
              <p className="card-sub" style={{ margin: '8px 0' }}>{f.description || 'بدون توضیح'}</p>

              {/* Visual Pipeline */}
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap', margin: '14px 0', background: 'var(--bg-subtle, #0f172a)', padding: '12px', borderRadius: '8px' }}>
                {f.steps?.map((step, idx) => (
                  <div key={idx} style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <div style={{ background: '#1e293b', border: '1px solid #334155', borderRadius: '6px', padding: '4px 10px', fontSize: '0.85rem' }}>
                      <div style={{ color: '#94a3b8', fontSize: '0.7rem' }}>گام {idx + 1}</div>
                      <span className="mono" style={{ color: '#e2e8f0' }}>{step.agent}</span>
                    </div>
                    {idx < f.steps.length - 1 && <span style={{ color: '#64748b' }}>➔</span>}
                  </div>
                ))}
              </div>

              <div style={{ display: 'flex', gap: '8px', marginTop: '16px' }}>
                <button
                  className="btn btn-sm btn-secondary"
                  style={{ flex: 1 }}
                  onClick={() => setEditingFlow({ ...f })}
                >
                  ✏️ ویرایش فلو
                </button>
                <button
                  className="btn btn-sm btn-primary"
                  onClick={() => {
                    setTestTarget(f.name);
                    setTestType('flow');
                    setTab('playground');
                  }}
                >
                  ⚡ تست زنده
                </button>
                <button
                  className="btn btn-sm btn-danger"
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
        <div className="card" style={{ maxWidth: '800px', margin: '0 auto' }}>
          <h3 style={{ marginBottom: '16px' }}>⚡ پلی‌گراند اجرای زنده</h3>
          <form onSubmit={handleRunTest}>
            <div style={{ display: 'flex', gap: '12px', marginBottom: '16px' }}>
              <div style={{ flex: 1 }}>
                <label className="label">نوع هدف</label>
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
                <label className="label">نام ایجنت / فلو</label>
                <select
                  className="input"
                  value={testTarget}
                  onChange={(e) => setTestTarget(e.target.value)}
                >
                  {testType === 'flow'
                    ? flows.map((f) => <option key={f.name} value={f.name}>{f.name}</option>)
                    : agents.map((a) => <option key={a.name} value={a.name}>{a.name}</option>)}
                </select>
              </div>
            </div>

            <div style={{ marginBottom: '16px' }}>
              <label className="label">ورودی / متن مقاله / پرامپت</label>
              <textarea
                className="input"
                rows={5}
                placeholder="متن خود را برای ارسال به ایجنت یا فلو وارد کنید..."
                value={testPrompt}
                onChange={(e) => setTestPrompt(e.target.value)}
                required
              />
            </div>

            <button type="submit" className="btn btn-primary" disabled={testRunning} style={{ width: '100%' }}>
              {testRunning ? '⏳ در حال اجرا و پردازش...' : '🚀 ارسال و اجرای آنی'}
            </button>
          </form>

          {testResponse && (
            <div style={{ marginTop: '24px', borderTop: '1px solid var(--border)', paddingTop: '16px' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '8px' }}>
                <h4>پاسخ دریافتی:</h4>
                <button
                  className="btn btn-sm btn-secondary"
                  onClick={() => navigator.clipboard.writeText(testResponse)}
                >
                  📋 کپی پاسخ
                </button>
              </div>
              <pre style={{ background: '#0b1329', padding: '16px', borderRadius: '8px', overflow: 'auto', maxHeight: '400px', whiteSpace: 'pre-wrap', direction: 'rtl', textAlign: 'right', fontSize: '0.9rem', color: '#e2e8f0', border: '1px solid #1e293b' }}>
                {testResponse}
              </pre>
            </div>
          )}
        </div>
      )}

      {/* EDIT AGENT MODAL */}
      {editingAgent && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000, padding: '20px' }}>
          <div className="card" style={{ maxWidth: '640px', width: '100%', maxHeight: '90vh', overflowY: 'auto' }}>
            <h3>{editingAgent.name ? `ویرایش ساب‌اجنت ${editingAgent.name}` : 'ساخت ساب‌اجنت جدید'}</h3>
            <form onSubmit={handleSaveAgent} style={{ marginTop: '16px' }}>
              <div style={{ marginBottom: '12px' }}>
                <label className="label">نام ایجنت (Slug)</label>
                <input
                  type="text"
                  className="input mono"
                  value={editingAgent.name}
                  onChange={(e) => setEditingAgent({ ...editingAgent, name: e.target.value })}
                  required
                />
              </div>
              <div style={{ marginBottom: '12px' }}>
                <label className="label">توضیحات</label>
                <input
                  type="text"
                  className="input"
                  value={editingAgent.description || ''}
                  onChange={(e) => setEditingAgent({ ...editingAgent, description: e.target.value })}
                />
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px', marginBottom: '12px' }}>
                <div>
                  <label className="label">مدل پایه (Model Alias)</label>
                  <select
                    className="input"
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
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000, padding: '20px' }}>
          <div className="card" style={{ maxWidth: '680px', width: '100%', maxHeight: '90vh', overflowY: 'auto' }}>
            <h3>{editingFlow.name ? `ویرایش خط‌لوله ${editingFlow.name}` : 'ساخت خط‌لوله جدید'}</h3>
            <form onSubmit={handleSaveFlow} style={{ marginTop: '16px' }}>
              <div style={{ marginBottom: '12px' }}>
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

              <div style={{ marginBottom: '16px' }}>
                <label className="label">گام‌های خط‌لوله (Steps)</label>
                {editingFlow.steps?.map((st, i) => (
                  <div key={i} style={{ display: 'flex', gap: '8px', alignItems: 'center', marginBottom: '8px', background: '#0b1329', padding: '8px 12px', borderRadius: '6px' }}>
                    <span style={{ fontSize: '0.8rem', color: '#94a3b8' }}>گام {i + 1}:</span>
                    <select
                      className="input"
                      style={{ flex: 2 }}
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
                      style={{ flex: 2 }}
                      value={st.label || ''}
                      onChange={(e) => {
                        const newSteps = [...editingFlow.steps];
                        newSteps[i].label = e.target.value;
                        setEditingFlow({ ...editingFlow, steps: newSteps });
                      }}
                    />
                    <button
                      type="button"
                      className="btn btn-sm btn-danger"
                      onClick={() => {
                        const newSteps = editingFlow.steps.filter((_, idx) => idx !== i);
                        setEditingFlow({ ...editingFlow, steps: newSteps });
                      }}
                    >
                      ❌
                    </button>
                  </div>
                ))}
                <button
                  type="button"
                  className="btn btn-sm btn-secondary"
                  style={{ marginTop: '8px' }}
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
