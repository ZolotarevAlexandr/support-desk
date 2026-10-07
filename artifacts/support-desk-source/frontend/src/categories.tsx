import { useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { api, type Category } from "./api";
import { useAuth } from "./auth";
import { useCategories } from "./tickets";
import { Confirm, ErrorMessage, Loading } from "./shared";
import styles from "./App.module.css";
export function Categories() {
  const { user } = useAuth();
  const categories = useCategories();
  const qc = useQueryClient();
  const [edit, setEdit] = useState<Category | null>(null);
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  if (user?.role !== "operator") return <Navigate to="/tickets" replace />;
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    setBusy(true);
    setError(undefined);
    try {
      await api(
        edit ? "/categories/" + edit.id : "/categories",
        edit ? "PATCH" : "POST",
        Object.fromEntries(new FormData(form)),
      );
      setEdit(null);
      form.reset();
      await qc.invalidateQueries({ queryKey: ["categories"] });
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <h1>Categories</h1>
      <p className={styles.muted}>
        Keep requests organized by the kind of help people need.
      </p>
      <div className={styles.twoCols}>
        <section className={styles.card}>
          <h2>{edit ? "Edit category" : "New category"}</h2>
          <form key={edit?.id || "new"} onSubmit={save}>
            <label>
              Name
              <input
                name="name"
                required
                maxLength={100}
                defaultValue={edit?.name}
              />
            </label>
            <label>
              Description
              <textarea
                name="description"
                maxLength={1000}
                defaultValue={edit?.description}
              />
            </label>
            <ErrorMessage error={error} />
            <div className={styles.actions}>
              <button className={styles.primary} disabled={busy}>
                {busy ? "Saving…" : edit ? "Save category" : "Create category"}
              </button>
              {edit && (
                <button type="button" onClick={() => setEdit(null)}>
                  Cancel
                </button>
              )}
            </div>
          </form>
        </section>
        <section className={styles.card}>
          <h2>Available categories</h2>
          <ErrorMessage error={categories.error} />
          {categories.isPending ? (
            <Loading />
          ) : categories.data?.length ? (
            categories.data.map((c) => (
              <article key={c.id} className={styles.comment}>
                <h3>{c.name}</h3>
                <p>{c.description}</p>
                <div className={styles.actions}>
                  <button
                    onClick={() => {
                      setEdit(c);
                      setError(undefined);
                    }}
                  >
                    Edit category
                  </button>
                  <Confirm
                    title="Delete category"
                    onConfirm={async () => {
                      await api("/categories/" + c.id, "DELETE");
                      if (edit?.id === c.id) setEdit(null);
                      await qc.invalidateQueries({ queryKey: ["categories"] });
                    }}
                  />
                </div>
              </article>
            ))
          ) : (
            <p>No categories yet. Create the first one.</p>
          )}
        </section>
      </div>
    </>
  );
}
