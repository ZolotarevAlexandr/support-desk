import { useState, type FormEvent } from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Category, type Ticket, type Page } from "./api";
import { useAuth } from "./auth";
import {
  Badge,
  ErrorMessage,
  Loading,
  Pager,
  statuses,
  priorities,
  label,
} from "./shared";
import { Comments } from "./comments";
import { ClaimTicketButton, TicketWorkflow } from "./workflow";
import styles from "./App.module.css";
export function useCategories() {
  return useQuery({
    queryKey: ["categories"],
    queryFn: () => api<Category[]>("/categories"),
  });
}
export function TicketList() {
  const { user } = useAuth();
  const [params, setParams] = useSearchParams();
  const categories = useCategories();
  const [claimError, setClaimError] = useState<unknown>();
  const page = Number(params.get("page") || 1);
  const tickets = useQuery({
    queryKey: ["tickets", params.toString()],
    queryFn: () => api<Page<Ticket>>("/tickets?" + params.toString()),
  });
  function filter(k: string, v: string) {
    const p = new URLSearchParams(params);
    if (v) p.set(k, v);
    else p.delete(k);
    p.delete("page");
    setParams(p);
  }
  return (
    <>
      <div className={styles.heading}>
        <div>
          <p className={styles.eyebrow}>SUPPORT WORKSPACE</p>
          <h1>{user?.role === "operator" ? "Operator queue" : "My tickets"}</h1>
          <p className={styles.muted}>
            Track requests, share updates, and find a resolution.
          </p>
        </div>
        <Link className={styles.primary} to="/tickets/new">
          + Create ticket
        </Link>
      </div>
      {user?.role === "operator" && (
        <div className={styles.actions}>
          {[
            ["", "All tickets"],
            ["me", "Assigned to me"],
            ["unassigned", "Unassigned"],
          ].map(([v, t]) => (
            <button
              key={v}
              aria-pressed={(params.get("assignment") || "") === v}
              onClick={() => filter("assignment", v)}
            >
              {t}
            </button>
          ))}
        </div>
      )}
      <section className={styles.card}>
        <div className={styles.filters}>
          <label>
            Search
            <input
              type="search"
              placeholder="Search title or description"
              value={params.get("search") || ""}
              onChange={(e) => filter("search", e.target.value)}
            />
          </label>
          <label>
            Status
            <select
              value={params.get("status") || ""}
              onChange={(e) => filter("status", e.target.value)}
            >
              <option value="">All statuses</option>
              {statuses.map((v) => (
                <option key={v} value={v}>
                  {label(v)}
                </option>
              ))}
            </select>
          </label>
          <label>
            Priority
            <select
              value={params.get("priority") || ""}
              onChange={(e) => filter("priority", e.target.value)}
            >
              <option value="">All priorities</option>
              {priorities.map((v) => (
                <option key={v} value={v}>
                  {label(v)}
                </option>
              ))}
            </select>
          </label>
          <label>
            Category
            <select
              value={params.get("category_id") || ""}
              onChange={(e) => filter("category_id", e.target.value)}
            >
              <option value="">All categories</option>
              {categories.data?.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.name}
                </option>
              ))}
            </select>
          </label>
        </div>
        <ErrorMessage error={claimError || categories.error || tickets.error} />
        {tickets.isPending ? (
          <Loading />
        ) : (
          tickets.data && (
            <>
              {!tickets.data.items.length ? (
                <div className={styles.empty}>
                  <h2>No tickets found</h2>
                  <p>Create a request or adjust your filters.</p>
                </div>
              ) : (
                <div className={styles.tableWrap}>
                  <table>
                    <thead>
                      <tr>
                        <th>Ticket</th>
                        <th>Status</th>
                        <th>Priority</th>
                        <th>Category</th>
                        <th>Created</th>
                        <th>Assigned to</th>
                        {user?.role === "operator" && <th>Action</th>}
                      </tr>
                    </thead>
                    <tbody>
                      {tickets.data.items.map((t) => (
                        <tr key={t.id}>
                          <td>
                            <Link to={"/tickets/" + t.id}>
                              <small>#{t.id}</small> {t.title}
                            </Link>
                          </td>
                          <td>
                            <Badge value={t.status} />
                          </td>
                          <td>
                            <Badge value={t.priority} />
                          </td>
                          <td>
                            {categories.data?.find(
                              (c) => c.id === t.category_id,
                            )?.name || "—"}
                          </td>
                          <td>{new Date(t.created_at).toLocaleDateString()}</td>
                          <td>{t.assignee_name || "Unassigned"}</td>
                          {user?.role === "operator" && (
                            <td>
                              {t.assignee_id === null &&
                                ["open", "in_progress"].includes(t.status) && (
                                  <ClaimTicketButton
                                    ticket={t}
                                    onError={setClaimError}
                                  />
                                )}
                            </td>
                          )}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              <Pager
                page={page}
                total={tickets.data.total}
                size={tickets.data.page_size}
                onChange={(v) => {
                  const p = new URLSearchParams(params);
                  p.set("page", String(v));
                  setParams(p);
                }}
              />
            </>
          )
        )}
      </section>
    </>
  );
}
export function TicketForm({
  ticket,
  onSaved,
  onCancel,
}: {
  ticket?: Ticket;
  onSaved: (ticket: Ticket) => void;
  onCancel?: () => void;
}) {
  const categories = useCategories();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setBusy(true);
    setError(undefined);
    const data = Object.fromEntries(new FormData(e.currentTarget));
    try {
      const t = await api<Ticket>(
        ticket ? "/tickets/" + ticket.id : "/tickets",
        ticket ? "PATCH" : "POST",
        { ...data, category_id: Number(data.category_id) },
      );
      onSaved(t);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <form onSubmit={submit}>
      <label>
        Title
        <input
          name="title"
          required
          maxLength={200}
          defaultValue={ticket?.title}
          placeholder="A short summary of the issue"
        />
      </label>
      <label>
        Description
        <textarea
          name="description"
          required
          maxLength={10000}
          rows={6}
          defaultValue={ticket?.description}
          placeholder="What happened? What have you tried?"
        />
      </label>
      <div className={styles.twoCols}>
        <label>
          Category
          <select
            name="category_id"
            required
            defaultValue={ticket?.category_id || ""}
          >
            <option value="" disabled>
              Choose a category
            </option>
            {categories.data?.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Priority
          <select name="priority" defaultValue={ticket?.priority || "medium"}>
            {priorities.map((p) => (
              <option key={p} value={p}>
                {label(p)}
              </option>
            ))}
          </select>
        </label>
      </div>
      <ErrorMessage error={error || categories.error} />
      {categories.data?.length === 0 && (
        <p role="status">
          An operator must create a category before you can submit a ticket.
        </p>
      )}
      <div className={styles.actions}>
        <button
          className={styles.primary}
          disabled={busy || !categories.data?.length}
        >
          {busy ? "Saving…" : ticket ? "Save changes" : "Submit ticket"}
        </button>
        {onCancel && (
          <button type="button" onClick={onCancel}>
            Cancel
          </button>
        )}
      </div>
    </form>
  );
}
export function NewTicket() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  return (
    <>
      <Link to="/tickets">← Tickets</Link>
      <h1>Create a ticket</h1>
      <section className={styles.card}>
        <TicketForm
          onSaved={(t) => {
            void qc.invalidateQueries({ queryKey: ["tickets"] });
            navigate("/tickets/" + t.id);
          }}
        />
      </section>
    </>
  );
}
export function TicketDetail() {
  const { id } = useParams();
  const { user } = useAuth();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [editing, setEditing] = useState(false);
  const categories = useCategories();
  const ticket = useQuery({
    queryKey: ["ticket", id],
    queryFn: () => api<Ticket>("/tickets/" + id),
  });
  if (ticket.isPending) return <Loading />;
  if (!ticket.data)
    return (
      <>
        <Link to="/tickets">← Tickets</Link>
        <ErrorMessage error={ticket.error} />
      </>
    );
  const t = ticket.data;
  const canEdit =
    user?.role === "operator" ||
    (t.requester_id === user?.id && t.status === "open");
  return (
    <>
      <Link to="/tickets">← Tickets</Link>
      <div className={styles.heading}>
        <div>
          <p className={styles.eyebrow}>REQUEST #{t.id}</p>
          <h1>{t.title}</h1>
          <div className={styles.actions}>
            <Badge value={t.status} />
            <Badge value={t.priority} />
            <span className={styles.muted}>
              {categories.data?.find((c) => c.id === t.category_id)?.name}
            </span>
          </div>
        </div>
      </div>
      <ErrorMessage error={ticket.error} />
      <section className={styles.card}>
        {editing && canEdit ? (
          <TicketForm
            ticket={t}
            onCancel={() => setEditing(false)}
            onSaved={(v) => {
              qc.setQueryData(["ticket", id], v);
              void qc.invalidateQueries({ queryKey: ["tickets"] });
              setEditing(false);
            }}
          />
        ) : (
          <>
            <h2>Request details</h2>
            <small className={styles.muted}>
              Requested by {t.requester_name}
            </small>
            <p className={styles.plainText}>{t.description}</p>
            <small className={styles.muted}>
              Created {new Date(t.created_at).toLocaleString()} · Updated{" "}
              {new Date(t.updated_at).toLocaleString()}
            </small>
          </>
        )}
      </section>
      <div className={styles.ticketWorkspace}>
        <Comments ticket={t} />
        <TicketWorkflow
          ticket={t}
          canEdit={canEdit}
          onEdit={() => {
            setEditing(true);
            window.scrollTo({ top: 0, behavior: "smooth" });
          }}
          onDelete={async () => {
            await api("/tickets/" + id, "DELETE");
            void qc.invalidateQueries({ queryKey: ["tickets"] });
            qc.removeQueries({ queryKey: ["ticket", id] });
            qc.removeQueries({ queryKey: ["comments", id] });
            navigate("/tickets");
          }}
        />
      </div>
    </>
  );
}
