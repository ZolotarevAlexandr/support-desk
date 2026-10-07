import { useEffect, useRef, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Comment, type Page, type Ticket } from "./api";
import { useAuth } from "./auth";
import { Badge, Confirm, ErrorMessage, Loading, Pager } from "./shared";
import styles from "./App.module.css";
export function Comments({ ticket }: { ticket: Ticket }) {
  const { user } = useAuth();
  const qc = useQueryClient();
  const [page, setPage] = useState(1);
  const [editing, setEditing] = useState<number | null>(null);
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const comments = useQuery({
    queryKey: ["comments", String(ticket.id), page],
    queryFn: () =>
      api<Page<Comment>>(`/tickets/${ticket.id}/comments?page=${page}`),
  });
  // Jump to new activity once, while preserving deliberate navigation to older pages.
  const latestActivity = useRef<{ total?: number; updatedAt?: string }>({});
  useEffect(() => {
    if (!comments.data) return;
    if (
      latestActivity.current.total !== comments.data.total ||
      latestActivity.current.updatedAt !== ticket.updated_at
    ) {
      latestActivity.current = {
        total: comments.data.total,
        updatedAt: ticket.updated_at,
      };
      setPage(Math.max(1, Math.ceil(comments.data.total / 25)));
    }
  }, [comments.data?.total, ticket.updated_at]);
  const closed = ticket.status === "closed";
  const base = `/tickets/${ticket.id}/comments`;
  const invalidate = () =>
    qc.invalidateQueries({ queryKey: ["comments", String(ticket.id)] });
  async function submit(e: FormEvent<HTMLFormElement>, id?: number) {
    e.preventDefault();
    const form = e.currentTarget;
    const body = String(new FormData(form).get("body"));
    setBusy(true);
    setError(undefined);
    try {
      await api(id ? base + "/" + id : base, id ? "PATCH" : "POST", { body });
      form.reset();
      setEditing(null);
      if (!id)
        setPage(Math.max(1, Math.ceil(((comments.data?.total || 0) + 1) / 25)));
      await invalidate();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className={styles.card}>
      <h2>
        Conversation{" "}
        <small className={styles.muted}>
          {comments.data?.total || 0} messages
        </small>
      </h2>
      <ErrorMessage error={error || comments.error} />
      {comments.isPending ? (
        <Loading />
      ) : (
        comments.data && (
          <>
            {!comments.data.items.length && (
              <p className={styles.muted}>
                No messages yet. Add an update to start the conversation.
              </p>
            )}
            {comments.data.items.map((c) => (
              <article key={c.id} className={styles.comment}>
                <div className={styles.heading}>
                  <strong>
                    {c.author_name}{" "}
                    {c.author_role === "operator" && <Badge value="operator" />}
                  </strong>
                  <small>
                    {new Date(c.created_at).toLocaleString()}
                    {c.updated_at !== c.created_at ? " · edited" : ""}
                  </small>
                </div>
                {editing === c.id && !closed ? (
                  <form onSubmit={(e) => void submit(e, c.id)}>
                    <label>
                      Edit message
                      <textarea
                        name="body"
                        required
                        maxLength={5000}
                        defaultValue={c.body}
                      />
                    </label>
                    <div className={styles.actions}>
                      <button disabled={busy}>Save message</button>
                      <button type="button" onClick={() => setEditing(null)}>
                        Cancel
                      </button>
                    </div>
                  </form>
                ) : (
                  <p className={styles.plainText}>{c.body}</p>
                )}
                {!closed && (
                  <div className={styles.actions}>
                    {c.author_id === user?.id && (
                      <button onClick={() => setEditing(c.id)}>
                        Edit message
                      </button>
                    )}
                    {(c.author_id === user?.id ||
                      user?.role === "operator") && (
                      <Confirm
                        title="Delete message"
                        onConfirm={async () => {
                          await api(base + "/" + c.id, "DELETE");
                          await invalidate();
                        }}
                      />
                    )}
                  </div>
                )}
              </article>
            ))}
            <Pager
              page={page}
              total={comments.data.total}
              size={25}
              onChange={setPage}
            />
          </>
        )
      )}
      {closed ? (
        <p role="status">
          This ticket is closed. An operator can reopen it to continue the
          conversation.
        </p>
      ) : (
        <form onSubmit={(e) => void submit(e)}>
          <label>
            Add a message
            <textarea
              name="body"
              required
              maxLength={5000}
              rows={3}
              placeholder="Share an update or ask a question"
            />
          </label>
          <button className={styles.primary} disabled={busy}>
            {busy ? "Sending…" : "Send message"}
          </button>
        </form>
      )}
    </section>
  );
}
