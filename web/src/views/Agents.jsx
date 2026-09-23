import { useEffect, useState } from 'react';
import Layout from '../components/Layout.jsx';
import * as api from '../api.js';
import { fmtInt, useT } from '../i18n/index.jsx';
import Icon from '../components/Icon.jsx';

const T = {
  fa: {
    title: 'استودیو ایجنت‌ها و فلوها',
    subtitle: 'مدیریت، ویرایش زنده، ساخت تیم‌های چند ایجنتی و تست تعاملی',
    savedAgent: 'ساب‌اجنت «{name}» با موفقیت ذخیره شد.',
    confirmDeleteAgent: 'آیا از حذف ساب‌اجنت «{name}» اطمینان دارید؟',
    deletedAgent: 'ساب‌اجنت «{name}» حذف شد.',
    savedFlow: 'خط‌لوله «{name}» با موفقیت ذخیره شد.',
    confirmDeleteFlow: 'آیا از حذف خط‌لوله «{name}» اطمینان دارید؟',
    deletedFlow: 'خط‌لوله «{name}» حذف شد.',
    newFlow: 'ساخت خط‌لوله (Flow)',
    newAgent: 'ساخت ساب‌اجنت',
    defFlowDesc: 'توضیح خط‌لوله',
    defStep1: 'تحلیل اولیه',
    defStep2: 'بازبینی',
    defSystem: 'دستورات سیستمی ایجنت را اینجا وارد کنید.',
    tabAgents: 'ساب‌اجنت‌ها ({n})',
    tabFlows: 'خط‌لوله‌ها / Flows ({n})',
    tabPlayground: 'پلی‌گراند و تست زنده',
    searchAgents: 'جستجو در ساب‌اجنت‌ها...',
    all: 'همه',
    noDesc: 'بدون توضیح',
    edit: 'ویرایش',
    test: 'تست',
    delete: 'حذف',
    steps: 'مراحل (Steps):',
    runFlow: 'اجرای فلو',
    pgTitle: 'پلی‌گراند اجرای زنده',
    pgSub: 'اجرای سریع ایجنت‌ها و خط‌لوله‌ها برای تست پرامپت و خروجی.',
    targetType: 'نوع هدف (Target Type)',
    optFlow: 'خط‌لوله (Flow)',
    optAgent: 'ساب‌اجنت تکی (Sub-Agent)',
    targetName: 'نام ایجنت / خط‌لوله',
    inputLabel: 'ورودی / متن مقاله / پرامپت (Input)',
    inputPh: 'متن خود را برای ارسال به ایجنت یا فلو وارد کنید...',
    running: 'در حال اجرا و پردازش...',
    run: 'ارسال و اجرای آنی',
    response: 'پاسخ دریافتی:',
    copy: 'کپی متن',
    editAgentTitle: 'ویرایش ساب‌اجنت {name}',
    newAgentTitle: 'ساخت ساب‌اجنت جدید',
    agentSlug: 'نام ایجنت (Slug)',
    description: 'توضیحات',
    modelAlias: 'مدل پایه (Model Alias)',
    temperature: 'Temperature (دقت/خلاقیت)',
    systemPrompt: 'دستورات سیستمی (System Prompt)',
    cancel: 'انصراف',
    saveAgent: 'ذخیره ساب‌اجنت',
    editFlowTitle: 'ویرایش خط‌لوله {name}',
    newFlowTitle: 'ساخت خط‌لوله جدید',
    flowName: 'نام خط‌لوله (Flow Name)',
    flowSteps: 'گام‌های خط‌لوله (Steps)',
    stepTitlePh: 'عنوان گام',
    stepN: 'گام {n}',
    removeStep: 'حذف گام',
    addStep: 'افزودن گام جدید',
    saveFlow: 'ذخیره خط‌لوله',
  },
  en: {
    title: 'Agents & Flows Studio',
    subtitle: 'Manage and live-edit agents, build multi-agent teams and test them interactively',
    savedAgent: 'Sub-agent “{name}” saved.',
    confirmDeleteAgent: 'Delete sub-agent “{name}”?',
    deletedAgent: 'Sub-agent “{name}” deleted.',
    savedFlow: 'Flow “{name}” saved.',
    confirmDeleteFlow: 'Delete flow “{name}”?',
    deletedFlow: 'Flow “{name}” deleted.',
    newFlow: 'New flow',
    newAgent: 'New sub-agent',
    defFlowDesc: 'Flow description',
    defStep1: 'Initial analysis',
    defStep2: 'Review',
    defSystem: 'Enter the agent’s system instructions here.',
    tabAgents: 'Sub-agents ({n})',
    tabFlows: 'Flows ({n})',
    tabPlayground: 'Playground & live test',
    searchAgents: 'Search sub-agents...',
    all: 'All',
    noDesc: 'No description',
    edit: 'Edit',
    test: 'Test',
    delete: 'Delete',
    steps: 'Steps:',
    runFlow: 'Run flow',
    pgTitle: 'Live playground',
    pgSub: 'Run agents and flows on the spot to test prompts and output.',
    targetType: 'Target type',
    optFlow: 'Flow',
    optAgent: 'Single sub-agent',
    targetName: 'Agent / flow name',
    inputLabel: 'Input (article text / prompt)',
    inputPh: 'Enter the text to send to the agent or flow...',
    running: 'Running...',
    run: 'Send and run',
    response: 'Response:',
    copy: 'Copy',
    editAgentTitle: 'Edit sub-agent {name}',
    newAgentTitle: 'New sub-agent',
    agentSlug: 'Agent name (slug)',
    description: 'Description',
    modelAlias: 'Base model (alias)',
    temperature: 'Temperature (precision/creativity)',
    systemPrompt: 'System prompt',
    cancel: 'Cancel',
    saveAgent: 'Save sub-agent',
    editFlowTitle: 'Edit flow {name}',
    newFlowTitle: 'New flow',
    flowName: 'Flow name',
    flowSteps: 'Flow steps',
    stepTitlePh: 'Step title',
    stepN: 'Step {n}',
    removeStep: 'Remove step',
    addStep: 'Add step',
    saveFlow: 'Save flow',
  },
};

export default function Agents() {
  const t = useT(T);
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
      setSuccess(t('savedAgent', { name: editingAgent.name }));
      loadData();
      setTimeout(() => setSuccess(null), 3000);
    } catch (err) {
      setError(err.message);
    }
  }

  // Delete Agent
  async function handleDeleteAgent(name) {
    if (!confirm(t('confirmDeleteAgent', { name }))) return;
    try {
      await api.deleteAgent(name);
      setSuccess(t('deletedAgent', { name }));
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
      setSuccess(t('savedFlow', { name: editingFlow.name }));
      loadData();
      setTimeout(() => setSuccess(null), 3000);
    } catch (err) {
      setError(err.message);
    }
  }

  // Delete Flow
  async function handleDeleteFlow(name) {
    if (!confirm(t('confirmDeleteFlow', { name }))) return;
    try {
      await api.deleteFlow(name);
      setSuccess(t('deletedFlow', { name }));
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
      title={t('title')}
      subtitle={t('subtitle')}
      actions={
        <div style={{ display: 'flex', gap: '8px' }}>
          <button
            className="btn btn-secondary"
            onClick={() =>
              setEditingFlow({
                name: 'new-flow',
                description: t('defFlowDesc'),
                steps: [
                  { agent: 'seo-content-auditor', label: t('defStep1') },
                  { agent: 'seo-strategist-reviewer', label: t('defStep2') },
                ],
              })
            }
          >
            <Icon name="plus" size={16} />{t('newFlow')}
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
                system: t('defSystem'),
              })
            }
          >
            <Icon name="plus" size={16} />{t('newAgent')}
          </button>
        </div>
      }
    >
      {error && <div className="banner-error" style={{ marginBottom: '16px' }}>{error}</div>}
      {success && <div className="banner-success" style={{ marginBottom: '16px', background: 'var(--ng-success-bg)', border: '1px solid var(--ng-success)', color: 'var(--ng-success-text)', padding: '12px 16px', borderRadius: '8px' }}>{success}</div>}

      <div className="tabs-container">
        <div
          className={`tab-item ${tab === 'agents' ? 'active' : ''}`}
          onClick={() => setTab('agents')}
        >
          <Icon name="bot" size={16} /> {t('tabAgents', { n: fmtInt(agents.length) })}
        </div>
        <div
          className={`tab-item ${tab === 'flows' ? 'active' : ''}`}
          onClick={() => setTab('flows')}
        >
          <Icon name="flow" size={16} /> {t('tabFlows', { n: fmtInt(flows.length) })}
        </div>
        <div
          className={`tab-item ${tab === 'playground' ? 'active' : ''}`}
          onClick={() => setTab('playground')}
        >
          <Icon name="zap" size={16} /> {t('tabPlayground')}
        </div>
      </div>

      {/* TAB 1: AGENTS */}
      {tab === 'agents' && (
        <>
          <div style={{ display: 'flex', gap: '12px', marginBottom: '24px', flexWrap: 'wrap', alignItems: 'center' }}>
            <input
              type="text"
              placeholder={t('searchAgents')}
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
                  {sq === 'all' ? t('all') : sq.toUpperCase()}
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
                    {a.description || t('noDesc')}
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
                    ✏️ {t('edit')}
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
                    <Icon name="zap" size={15} />{t('test')}
                  </button>
                  <button
                    className="btn btn-ghost"
                    style={{ color: 'var(--ng-danger)', borderColor: 'transparent' }}
                    onClick={() => handleDeleteAgent(a.name)}
                    title={t('delete')}
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
                  {f.description || t('noDesc')}
                </p>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginTop: '16px', background: 'var(--ng-surface-soft)', padding: '12px', borderRadius: '8px' }}>
                  <strong style={{ fontSize: '11.5px', color: 'var(--ng-slate-700)' }}>{t('steps')}</strong>
                  {f.steps?.map((st, i) => (
                    <div key={i} style={{ display: 'flex', alignItems: 'center', gap: '8px', fontSize: '12.5px' }}>
                      <span style={{ width: '20px', height: '20px', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--ng-accent)', color: 'white', borderRadius: '50%', fontSize: '11px', fontWeight: 'bold' }}>{fmtInt(i + 1)}</span>
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
                  ✏️ {t('edit')}
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
                  <Icon name="zap" size={15} />{t('runFlow')}
                </button>
                <button
                  className="btn btn-ghost"
                  style={{ color: 'var(--ng-danger)', borderColor: 'transparent' }}
                  onClick={() => handleDeleteFlow(f.name)}
                  title={t('delete')}
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
              <Icon name="zap" size={20} />
            </div>
            <div>
              <h3 style={{ margin: 0, fontSize: '18px', color: 'var(--ng-heading)' }}>{t('pgTitle')}</h3>
              <p style={{ margin: '4px 0 0 0', fontSize: '12.5px', color: 'var(--ng-muted)' }}>{t('pgSub')}</p>
            </div>
          </div>

          <form onSubmit={handleRunTest}>
            <div style={{ display: 'flex', gap: '16px', marginBottom: '20px' }}>
              <div style={{ flex: 1 }}>
                <label className="label">{t('targetType')}</label>
                <select
                  className="input"
                  value={testType}
                  onChange={(e) => {
                    setTestType(e.target.value);
                    setTestTarget(e.target.value === 'flow' ? (flows[0]?.name || 'seo-audit-team') : (agents[0]?.name || 'seo-content-auditor'));
                  }}
                >
                  <option value="flow">{t('optFlow')}</option>
                  <option value="agent">{t('optAgent')}</option>
                </select>
              </div>
              <div style={{ flex: 2 }}>
                <label className="label">{t('targetName')}</label>
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
              <label className="label">{t('inputLabel')}</label>
              <textarea
                className="input"
                rows={6}
                placeholder={t('inputPh')}
                value={testPrompt}
                onChange={(e) => setTestPrompt(e.target.value)}
                required
              />
            </div>

            <button type="submit" className="btn btn-primary animated-btn" disabled={testRunning} style={{ width: '100%', fontSize: '14px', padding: '14px' }}>
              {testRunning ? <><Icon name="refresh" size={16} />{t('running')}</> : <><Icon name="zap" size={16} />{t('run')}</>}
            </button>
          </form>

          {testResponse && (
            <div style={{ marginTop: '32px', borderTop: '1px solid var(--ng-border-faint)', paddingTop: '24px', animation: 'fadeIn 0.4s ease-out' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '12px' }}>
                <h4 style={{ margin: 0, color: 'var(--ng-heading)', fontSize: '15px', display: 'flex', alignItems: 'center', gap: '8px' }}>
                  <span style={{ color: 'var(--ng-success-text)' }}>●</span> {t('response')}
                </h4>
                <button
                  className="btn btn-ghost"
                  onClick={() => navigator.clipboard.writeText(testResponse)}
                >
                  <Icon name="copy" size={15} />{t('copy')}
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
              {editingAgent.name ? t('editAgentTitle', { name: editingAgent.name }) : t('newAgentTitle')}
            </h3>
            <form onSubmit={handleSaveAgent}>
              <div style={{ marginBottom: '16px' }}>
                <label className="label">{t('agentSlug')}</label>
                <input
                  type="text"
                  className="input mono"
                  value={editingAgent.name}
                  onChange={(e) => setEditingAgent({ ...editingAgent, name: e.target.value })}
                  required
                />
              </div>
              <div style={{ marginBottom: '16px' }}>
                <label className="label">{t('description')}</label>
                <input
                  type="text"
                  className="input"
                  value={editingAgent.description || ''}
                  onChange={(e) => setEditingAgent({ ...editingAgent, description: e.target.value })}
                />
              </div>
              <div style={{ display: 'grid', gridTemplateColumns: '1.5fr 1fr', gap: '16px', marginBottom: '20px' }}>
                <div>
                  <label className="label">{t('modelAlias')}</label>
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
                  <label className="label">{t('temperature')}</label>
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
                <label className="label">{t('systemPrompt')}</label>
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
                  {t('cancel')}
                </button>
                <button type="submit" className="btn btn-primary">
                  <Icon name="check" size={15} />{t('saveAgent')}
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
              {editingFlow.name ? t('editFlowTitle', { name: editingFlow.name }) : t('newFlowTitle')}
            </h3>
            <form onSubmit={handleSaveFlow}>
              <div style={{ marginBottom: '16px' }}>
                <label className="label">{t('flowName')}</label>
                <input
                  type="text"
                  className="input mono"
                  value={editingFlow.name}
                  onChange={(e) => setEditingFlow({ ...editingFlow, name: e.target.value })}
                  required
                />
              </div>
              <div style={{ marginBottom: '16px' }}>
                <label className="label">{t('description')}</label>
                <input
                  type="text"
                  className="input"
                  value={editingFlow.description || ''}
                  onChange={(e) => setEditingFlow({ ...editingFlow, description: e.target.value })}
                />
              </div>

              <div style={{ marginBottom: '24px' }}>
                <label className="label" style={{ marginBottom: '12px' }}>{t('flowSteps')}</label>
                <div style={{ background: 'var(--ng-surface-soft)', padding: '16px', borderRadius: '12px', border: '1px solid var(--ng-border-faint)' }}>
                  {editingFlow.steps?.map((st, i) => (
                    <div key={i} style={{ display: 'flex', gap: '12px', alignItems: 'center', marginBottom: '12px' }}>
                      <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: '24px', height: '24px', background: 'var(--ng-surface)', borderRadius: '50%', color: 'var(--ng-muted)', fontSize: '11px', fontWeight: 'bold', border: '1px solid var(--ng-border-faint)' }}>
                        {fmtInt(i + 1)}
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
                        placeholder={t('stepTitlePh')}
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
                        style={{ padding: '8px', color: 'var(--ng-danger)' }}
                        title={t('removeStep')}
                        aria-label={t('removeStep')}
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
                        steps: [...(editingFlow.steps || []), { agent: agents[0]?.name || 'seo-content-auditor', label: t('stepN', { n: (editingFlow.steps?.length || 0) + 1 }) }],
                      })
                    }
                  >
                    <Icon name="plus" size={15} />{t('addStep')}
                  </button>
                </div>
              </div>

              <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
                <button type="button" className="btn btn-secondary" onClick={() => setEditingFlow(null)}>
                  {t('cancel')}
                </button>
                <button type="submit" className="btn btn-primary">
                  <Icon name="check" size={15} />{t('saveFlow')}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </Layout>
  );
}
