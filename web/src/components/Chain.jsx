// Renders an alias route as "primary → fallback fallback …", matching the
// design's chip chain. `note` replaces the fallback list (e.g. "بدون فالبک").
export default function Chain({ primary, fallbacks = [], note }) {
  return (
    <div className="chain">
      <span className="tag tag-primary ltr">{primary}</span>
      {note ? (
        <span className="none">{note}</span>
      ) : (
        fallbacks.length > 0 && (
          <>
            <span className="arrow" aria-hidden="true">
              →
            </span>
            {fallbacks.map((f) => (
              <span key={f} className="tag ltr">
                {f}
              </span>
            ))}
          </>
        )
      )}
    </div>
  );
}
