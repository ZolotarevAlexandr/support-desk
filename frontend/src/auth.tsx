import { createContext, useContext, useState, type FormEvent } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";
import { api, type User } from "./api";
import { ErrorMessage } from "./shared";
import styles from "./App.module.css";
export const AuthContext = createContext<{
  user: User | null;
  setUser: (u: User | null) => void;
}>({ user: null, setUser: () => {} });
export const useAuth = () => useContext(AuthContext);
export function AuthPage({ register = false }: { register?: boolean }) {
  const { user, setUser } = useAuth();
  const navigate = useNavigate();
  const [role, setRole] = useState("user");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  if (user) return <Navigate to="/tickets" replace />;
  async function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setError(undefined);
    const data = Object.fromEntries(new FormData(e.currentTarget));
    if (register && data.password !== data.password_confirmation) {
      setError(new Error("Passwords do not match"));
      return;
    }
    if (new TextEncoder().encode(String(data.password)).length > 72) {
      setError(new Error("Password must contain at most 72 bytes"));
      return;
    }
    setBusy(true);
    try {
      const u = await api<User>(
        "/auth/" + (register ? "register" : "login"),
        "POST",
        register ? { ...data, role } : data,
      );
      setUser(u);
      navigate("/tickets");
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <main className={styles.auth}>
      <div className={styles.brand}>
        S / D <span>Support Desk</span>
      </div>
      <section className={styles.card}>
        <p className={styles.eyebrow}>YOUR IT SUPPORT WORKSPACE</p>
        <h1>{register ? "Create your account" : "Welcome back"}</h1>
        <p className={styles.muted}>
          {register
            ? "Get help and keep every conversation in one place."
            : "Sign in to follow your requests and get things moving."}
        </p>
        <form onSubmit={submit}>
          {register && (
            <label>
              Name
              <input name="name" required maxLength={100} autoComplete="name" />
            </label>
          )}
          <label>
            Email
            <input
              name="email"
              type="email"
              required
              maxLength={254}
              autoComplete="email"
            />
          </label>
          <label>
            Password
            <input
              name="password"
              type="password"
              required
              minLength={register ? 12 : undefined}
              autoComplete={register ? "new-password" : "current-password"}
            />
          </label>
          {register && (
            <>
              <small>At least 12 characters, at most 72 bytes.</small>
              <label>
                Confirm password
                <input
                  name="password_confirmation"
                  type="password"
                  required
                  autoComplete="new-password"
                />
              </label>
              <label>
                Account type
                <select
                  name="role"
                  value={role}
                  onChange={(e) => setRole(e.target.value)}
                >
                  <option value="user">User</option>
                  <option value="operator">Operator</option>
                </select>
              </label>
              {role === "operator" && (
                <label>
                  Invitation code
                  <input
                    name="invite_code"
                    type="password"
                    required
                    autoComplete="off"
                  />
                  <small>Ask your team for an operator invitation.</small>
                </label>
              )}
            </>
          )}
          <ErrorMessage error={error} />
          <button className={styles.primary} disabled={busy}>
            {busy ? "Please wait…" : register ? "Create account" : "Sign in"}
          </button>
        </form>
        <p>
          {register ? "Already registered?" : "New here?"}{" "}
          <Link to={register ? "/login" : "/register"}>
            {register ? "Sign in" : "Create an account"}
          </Link>
        </p>
      </section>
    </main>
  );
}
