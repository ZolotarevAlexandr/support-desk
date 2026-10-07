import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, APIError, type Ticket, type User } from "./api";
import { useAuth } from "./auth";
import { Badge, Confirm, ErrorMessage, Loading, Modal } from "./shared";
import styles from "./App.module.css";
export type TicketAction = {
  action:
    | "claim"
    | "reassign"
    | "resolve"
    | "confirm"
    | "request_changes"
    | "reopen";
  message?: string;
  assignee_id?: number | null;
};
export function useTicketAction(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (action: TicketAction) =>
      api<Ticket>(`/tickets/${id}/actions`, "POST", action),
    onSuccess: async (ticket) => {
      qc.setQueryData(["ticket", String(id)], ticket);
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["tickets"] }),
        qc.invalidateQueries({ queryKey: ["comments", String(id)] }),
      ]);
    },
    onError: (error) => {
      if (error instanceof APIError && error.status === 409) {
        void qc.invalidateQueries({ queryKey: ["ticket", String(id)] });
        void qc.invalidateQueries({ queryKey: ["tickets"] });
      }
    },
  });
}
export function ClaimTicketButton({
  ticket,
  onError,
}: {
  ticket: Ticket;
  onError: (error: unknown) => void;
}) {
  const mutation = useTicketAction(ticket.id);
  return (
    <button
      aria-label={`Take ticket #${ticket.id}`}
      disabled={mutation.isPending}
      onClick={() => {
        onError(undefined);
        mutation.mutate({ action: "claim" }, { onError });
      }}
    >
      {mutation.isPending ? "Taking…" : "Take ticket"}
    </button>
  );
}
export function TicketWorkflow({
  ticket,
  canEdit,
  onEdit,
  onDelete,
}: {
  ticket: Ticket;
  canEdit: boolean;
  onEdit: () => void;
  onDelete: () => Promise<unknown>;
}) {
  const { user } = useAuth();
  const mutation = useTicketAction(ticket.id);
  const [mode, setMode] = useState<TicketAction["action"] | null>(null);
  const [search, setSearch] = useState("");
  const [selected, setSelected] = useState<number | null>(ticket.assignee_id);
  const operators = useQuery({
    queryKey: ["operators"],
    queryFn: () => api<User[]>("/operators"),
    enabled: user?.role === "operator" && mode === "reassign",
  });
  const active = ticket.status === "open" || ticket.status === "in_progress";
  const requester = ticket.requester_id === user?.id;
  const operator = user?.role === "operator";
  function open(action: TicketAction["action"]) {
    mutation.reset();
    setSelected(ticket.assignee_id);
    setSearch("");
    setMode(action);
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const message = String(
      new FormData(event.currentTarget).get("message") || "",
    ).trim();
    try {
      await mutation.mutateAsync(
        mode === "reassign"
          ? { action: mode, assignee_id: selected }
          : { action: mode as TicketAction["action"], message },
      );
      setMode(null);
    } catch {
      /* Keep input and the server error available for retry. */
    }
  }
  const captions = {
    resolve: {
      title: "Resolve ticket",
      field: "Resolution message",
      submit: "Mark resolved",
      help: "Explain what was fixed. The requester will confirm whether the issue is resolved.",
    },
    request_changes: {
      title: "Still need help",
      field: "What still needs attention?",
      submit: "Send back to operator",
      help: "Describe what is still wrong. This returns the ticket to work with its current operator.",
    },
    reopen: {
      title: "Reopen ticket",
      field: "Reason for reopening",
      submit: "Reopen ticket",
      help: "Explain why the conversation needs to continue.",
    },
  };
  const noteMode =
    mode === "resolve" || mode === "request_changes" || mode === "reopen"
      ? captions[mode]
      : null;
  return (
    <section
      className={`${styles.card} ${styles.workflow}`}
      aria-label="Request management"
    >
      <h2>Next step</h2>
      <div className={styles.actions}>
        <Badge value={ticket.status} />
      </div>
      <p className={styles.muted}>
        {ticket.assignee_id ? (
          <>
            Assigned to{" "}
            <strong>
              {ticket.assignee_name || `Operator #${ticket.assignee_id}`}
            </strong>
            {ticket.assignee_id === user?.id ? " (you)" : ""}
          </>
        ) : (
          "Waiting for an operator"
        )}
      </p>
      {operator && active && (
        <div className={styles.actions}>
          {ticket.assignee_id === null ? (
            <button
              className={styles.primary}
              disabled={mutation.isPending}
              onClick={() => mutation.mutate({ action: "claim" })}
            >
              {mutation.isPending ? "Taking…" : "Take ticket"}
            </button>
          ) : ticket.status === "open" ? (
            <button
              className={styles.primary}
              disabled={mutation.isPending}
              onClick={() =>
                mutation.mutate({
                  action: "reassign",
                  assignee_id: ticket.assignee_id,
                })
              }
            >
              {mutation.isPending ? "Starting…" : "Start work"}
            </button>
          ) : (
            <button
              className={styles.primary}
              disabled={mutation.isPending}
              onClick={() => open("resolve")}
            >
              Resolve…
            </button>
          )}
          <button
            disabled={mutation.isPending}
            onClick={() => open("reassign")}
          >
            Reassign…
          </button>
        </div>
      )}
      {ticket.status === "resolved" && (
        <>
          <p>
            {requester
              ? "Please check the resolution and let us know whether it worked."
              : "Waiting for the requester to confirm the resolution."}
          </p>
          {requester && (
            <div className={styles.actions}>
              <button
                className={styles.primary}
                disabled={mutation.isPending}
                onClick={() => mutation.mutate({ action: "confirm" })}
              >
                {mutation.isPending ? "Confirming…" : "Issue fixed"}
              </button>
              <button
                disabled={mutation.isPending}
                onClick={() => open("request_changes")}
              >
                Still need help
              </button>
            </div>
          )}
        </>
      )}
      {ticket.status === "closed" && (
        <p>
          This request is closed.{" "}
          {operator
            ? "Reopen from More actions if further work is needed."
            : "This conversation is read-only."}
        </p>
      )}
      {!operator && active && (
        <p>
          {ticket.status === "open"
            ? "An operator will pick up your request. Add any useful details below."
            : "Your request is being worked on. Use the conversation to share updates."}
        </p>
      )}
      <ErrorMessage error={!mode && mutation.error} />
      {(canEdit || (operator && !active)) && (
        <details className={styles.moreActions}>
          <summary>More actions</summary>
          <div className={styles.menuActions}>
            {canEdit && (
              <button disabled={mutation.isPending} onClick={onEdit}>
                Edit ticket
              </button>
            )}
            {operator && !active && (
              <button
                disabled={mutation.isPending}
                onClick={() => open("reopen")}
              >
                Reopen…
              </button>
            )}
            {canEdit && <Confirm title="Delete ticket" onConfirm={onDelete} />}
          </div>
        </details>
      )}
      {mode && (
        <Modal
          title={noteMode?.title || "Reassign ticket"}
          busy={mutation.isPending}
          onClose={() => {
            setMode(null);
            mutation.reset();
          }}
        >
          <form onSubmit={(event) => void submit(event)}>
            {noteMode ? (
              <>
                <p className={styles.muted}>{noteMode.help}</p>
                <label>
                  {noteMode.field}
                  <textarea
                    autoFocus
                    name="message"
                    required
                    maxLength={5000}
                    rows={5}
                  />
                </label>
              </>
            ) : (
              <>
                <label>
                  Find an operator
                  <input
                    autoFocus
                    type="search"
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    placeholder="Search by name or email"
                  />
                </label>
                <div className={styles.assignmentChoices}>
                  <button
                    type="button"
                    aria-pressed={selected === user?.id}
                    disabled={mutation.isPending}
                    onClick={() => setSelected(user!.id)}
                  >
                    Assign to me
                  </button>
                  <button
                    type="button"
                    aria-pressed={selected === null}
                    disabled={mutation.isPending}
                    onClick={() => setSelected(null)}
                  >
                    Return to queue
                  </button>
                </div>
                <p className={styles.muted}>
                  Selected:{" "}
                  {selected === null
                    ? "Unassigned"
                    : operators.data?.find((u) => u.id === selected)?.name ||
                      ticket.assignee_name ||
                      "Loading…"}
                </p>
                {operators.isPending ? (
                  <Loading />
                ) : (
                  <div className={styles.operatorList}>
                    {operators.data
                      ?.filter((u) =>
                        (u.name + " " + u.email)
                          .toLowerCase()
                          .includes(search.toLowerCase()),
                      )
                      .map((u) => (
                        <button
                          type="button"
                          key={u.id}
                          aria-pressed={selected === u.id}
                          disabled={mutation.isPending}
                          onClick={() => setSelected(u.id)}
                        >
                          <strong>
                            {u.name}
                            {u.id === user?.id ? " (you)" : ""}
                          </strong>
                          <small>{u.email}</small>
                        </button>
                      ))}
                    {operators.data &&
                      !operators.data.some((u) =>
                        (u.name + " " + u.email)
                          .toLowerCase()
                          .includes(search.toLowerCase()),
                      ) && <p>No operators match your search.</p>}
                  </div>
                )}
              </>
            )}
            <ErrorMessage
              error={mutation.error || (!noteMode && operators.error)}
            />
            <div className={styles.actions}>
              <button
                type="button"
                disabled={mutation.isPending}
                onClick={() => {
                  setMode(null);
                  mutation.reset();
                }}
              >
                Cancel
              </button>
              <button
                className={styles.primary}
                disabled={
                  mutation.isPending ||
                  (!noteMode && (operators.isPending || !!operators.error))
                }
              >
                {mutation.isPending
                  ? "Saving…"
                  : noteMode?.submit || "Save assignment"}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </section>
  );
}
