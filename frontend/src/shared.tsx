import { useEffect, useRef, useState } from "react";
import styles from "./App.module.css";
export const statuses = ["open", "in_progress", "resolved", "closed"];
export const priorities = ["low", "medium", "high", "urgent"];
export const label = (s: string) =>
  s.replaceAll("_", " ").replace(/^./, (c) => c.toUpperCase());
export function Badge({ value }: { value: string }) {
  return (
    <span className={`${styles.badge} ${styles[value] || ""}`}>
      {label(value)}
    </span>
  );
}
export function ErrorMessage({ error }: { error: unknown }) {
  return error ? (
    <p role="alert" className={styles.error}>
      {error instanceof Error ? error.message : "Something went wrong"}
    </p>
  ) : null;
}
export function Loading() {
  return <p role="status">Loading…</p>;
}
export function Pager({
  page,
  total,
  size,
  onChange,
}: {
  page: number;
  total: number;
  size: number;
  onChange: (page: number) => void;
}) {
  return (
    <div className={styles.pager}>
      <span>
        {total} results · Page {page} of {Math.max(1, Math.ceil(total / size))}
      </span>
      <button disabled={page <= 1} onClick={() => onChange(page - 1)}>
        Previous
      </button>
      <button
        disabled={page * size >= total}
        onClick={() => onChange(page + 1)}
      >
        Next
      </button>
    </div>
  );
}
export function Confirm({
  title,
  onConfirm,
}: {
  title: string;
  onConfirm: () => Promise<unknown>;
}) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<unknown>();
  const [working, setWorking] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (!open) return;
    const node = dialog.current;
    node?.showModal();
    return () => node?.close();
  }, [open]);
  return (
    <>
      <button
        type="button"
        className={styles.danger}
        onClick={() => {
          setError(undefined);
          setOpen(true);
        }}
      >
        {title}
      </button>
      {open && (
        <dialog
          ref={dialog}
          onCancel={(event) => {
            if (working) event.preventDefault();
            else setOpen(false);
          }}
          aria-modal="true"
          aria-label={title}
          className={styles.dialog}
        >
          <h2>{title}?</h2>
          <p>
            This permanently removes the record and any associated comments.
          </p>
          <ErrorMessage error={error} />
          <div className={styles.actions}>
            <button autoFocus disabled={working} onClick={() => setOpen(false)}>
              Cancel
            </button>
            <button
              className={styles.danger}
              disabled={working}
              onClick={async () => {
                setWorking(true);
                try {
                  await onConfirm();
                  setOpen(false);
                } catch (e) {
                  setError(e);
                } finally {
                  setWorking(false);
                }
              }}
            >
              {working ? "Deleting…" : "Confirm deletion"}
            </button>
          </div>
        </dialog>
      )}
    </>
  );
}

export function Modal({
  title,
  busy = false,
  onClose,
  children,
}: {
  title: string;
  busy?: boolean;
  onClose: () => void;
  children: import("react").ReactNode;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const node = dialog.current;
    node?.showModal();
    return () => node?.close();
  }, []);
  return (
    <dialog
      ref={dialog}
      aria-label={title}
      aria-modal="true"
      className={styles.dialog}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <h2>{title}</h2>
      {children}
    </dialog>
  );
}
