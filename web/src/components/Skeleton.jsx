/*
 * Loading placeholders that keep the page's shape while data arrives.
 *
 * A screen that renders "…" and then jumps into a table makes every load feel
 * like an error for a moment. Drawing the table's silhouette first means the
 * only thing that changes when data lands is the text.
 */

export function Skeleton({ w = '100%', h = 12, round = false, style }) {
  return (
    <span
      className={'sk' + (round ? ' sk-round' : '')}
      style={{ display: 'block', width: w, height: h, ...style }}
      aria-hidden="true"
    />
  );
}

export function SkeletonText({ lines = 3, widths = ['100%', '85%', '60%'] }) {
  return (
    <div className="sk-stack" aria-hidden="true">
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton key={i} w={widths[i % widths.length]} h={12} />
      ))}
    </div>
  );
}

export function SkeletonStats({ n = 4 }) {
  return (
    <div className="grid-auto" aria-hidden="true">
      {Array.from({ length: n }).map((_, i) => (
        <div key={i} className="card kpi">
          <Skeleton w="40%" h={12} />
          <Skeleton w="60%" h={26} />
          <Skeleton w="30%" h={10} />
        </div>
      ))}
    </div>
  );
}

export function SkeletonTable({ rows = 5, cols = 4 }) {
  return (
    <table className="tbl" aria-hidden="true" style={{ margin: 0 }}>
      <thead>
        <tr>
          {Array.from({ length: cols }).map((_, i) => (
            <th key={i}><Skeleton w={60 + ((i * 23) % 40) + '%'} h={10} /></th>
          ))}
        </tr>
      </thead>
      <tbody>
        {Array.from({ length: rows }).map((_, r) => (
          <tr key={r}>
            {Array.from({ length: cols }).map((_, c) => (
              <td key={c}><Skeleton w={45 + ((r * 17 + c * 29) % 50) + '%'} h={12} /></td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export function SkeletonCards({ n = 3, h = 120 }) {
  return (
    <div className="grid-auto" aria-hidden="true">
      {Array.from({ length: n }).map((_, i) => (
        <div key={i} className="card" style={{ minHeight: h }}>
          <div className="sk-stack">
            <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
              <Skeleton w={40} h={40} style={{ borderRadius: 10 }} />
              <div style={{ flex: 1 }} className="sk-stack">
                <Skeleton w="55%" h={14} />
                <Skeleton w="35%" h={10} />
              </div>
            </div>
            <Skeleton w="90%" h={10} />
            <Skeleton w="70%" h={10} />
          </div>
        </div>
      ))}
    </div>
  );
}

/* A full-page silhouette, shown before the session is even known. */
export function BootShell() {
  return (
    <div className="boot-shell" aria-busy="true" aria-label="در حال بارگذاری">
      <div className="boot-side">
        <div style={{ display: 'flex', gap: 10, alignItems: 'center', marginBottom: 12 }}>
          <Skeleton w={42} h={42} style={{ borderRadius: 12 }} />
          <div style={{ flex: 1 }} className="sk-stack"><Skeleton w="70%" h={14} /><Skeleton w="40%" h={10} /></div>
        </div>
        {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} w={(55 + (i * 13) % 35) + '%'} h={14} />)}
      </div>
      <div className="boot-main">
        <Skeleton w="30%" h={22} />
        <SkeletonStats n={4} />
        <div className="card"><SkeletonTable rows={5} cols={5} /></div>
      </div>
    </div>
  );
}
