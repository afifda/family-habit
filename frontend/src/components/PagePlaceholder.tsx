type PagePlaceholderProps = {
  title: string;
  description: string;
  eyebrow?: string;
};

export function PagePlaceholder({
  title,
  description,
  eyebrow = 'Phase 1 foundation',
}: PagePlaceholderProps) {
  return (
    <section className="page" aria-labelledby="page-title">
      <p className="eyebrow">{eyebrow}</p>
      <h1 id="page-title">{title}</h1>
      <p className="page-intro">{description}</p>
      <div className="empty-state" role="status">
        <span className="empty-state-icon" aria-hidden="true">
          ✓
        </span>
        <div>
          <h2>Ready for the next phase</h2>
          <p>This route is wired and waiting for its product workflow.</p>
        </div>
      </div>
    </section>
  );
}
