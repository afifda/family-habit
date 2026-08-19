import { ReactNode, useEffect, useRef } from 'react';

export function AccessibleDialog({
  titleId,
  onClose,
  backdropClassName = '',
  className = '',
  children,
}: {
  titleId: string;
  onClose: () => void;
  backdropClassName?: string;
  className?: string;
  children: ReactNode;
}) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const restoreRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    restoreRef.current = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current;
    const focusable = dialog?.querySelector<HTMLElement>(
      '[data-initial-focus], button, input, textarea, select, [href]',
    );
    focusable?.focus();
    return () => restoreRef.current?.focus();
  }, []);

  function onKeyDown(event: React.KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== 'Tab') return;
    const items = Array.from(
      dialogRef.current?.querySelectorAll<HTMLElement>(
        'button:not(:disabled), input:not(:disabled), textarea:not(:disabled), select:not(:disabled), [href]',
      ) ?? [],
    );
    if (!items.length) return;
    const first = items[0];
    const last = items.at(-1);
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last?.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first?.focus();
    }
  }

  return (
    <div
      className={['dialog-backdrop', backdropClassName]
        .filter(Boolean)
        .join(' ')}
      role="presentation"
    >
      <div
        ref={dialogRef}
        className={['dialog', className].filter(Boolean).join(' ')}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        onKeyDown={onKeyDown}
      >
        {children}
      </div>
    </div>
  );
}
