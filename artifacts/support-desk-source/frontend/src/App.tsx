import { useEffect, useState } from "react";
import {
  BrowserRouter,
  Link,
  NavLink,
  Navigate,
  Outlet,
  Route,
  Routes,
} from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { api, APIError, type User } from "./api";
import { AuthContext, AuthPage, useAuth } from "./auth";
import { NewTicket, TicketDetail, TicketList } from "./tickets";
import { Categories } from "./categories";
import { ErrorMessage, Loading } from "./shared";
import styles from "./App.module.css";
function Shell() {
  const { user, setUser } = useAuth();
  const qc = useQueryClient();
  const [error, setError] = useState<unknown>();
  if (!user) return <Navigate to="/login" replace />;
  return (
    <div className={styles.shell}>
      <aside className={styles.sidebar}>
        <Link className={styles.brand} to="/tickets">
          S / D <span>Support Desk</span>
        </Link>
        <p className={styles.eyebrow}>WORKSPACE</p>
        <nav>
          <NavLink to="/tickets" end>
            {user.role === "operator" ? "Operator queue" : "My tickets"}
          </NavLink>
          <NavLink to="/tickets/new">Create ticket</NavLink>
          {user.role === "operator" && (
            <NavLink to="/categories">Categories</NavLink>
          )}
        </nav>
        <div className={styles.profile}>
          <strong>{user.name}</strong>
          <small>
            {user.role === "operator" ? "Operator" : "User"} · {user.email}
          </small>
          <ErrorMessage error={error} />
          <button
            onClick={async () => {
              try {
                await api("/auth/logout", "POST");
                await qc.cancelQueries();
                qc.clear();
                setUser(null);
              } catch (e) {
                setError(e);
              }
            }}
          >
            Sign out
          </button>
        </div>
      </aside>
      <main className={styles.content}>
        <Outlet />
      </main>
    </div>
  );
}
export default function App() {
  const qc = useQueryClient();
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<unknown>();
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let active = true;
    setLoading(true);
    api<User>("/auth/me")
      .then((u) => {
        if (active) setUser(u);
      })
      .catch((e) => {
        if (active && !(e instanceof APIError && e.status === 401)) setError(e);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [attempt]);
  useEffect(() => {
    const expire = () => {
      void qc.cancelQueries();
      qc.clear();
      setUser(null);
    };
    window.addEventListener("session-expired", expire);
    return () => window.removeEventListener("session-expired", expire);
  }, [qc]);
  if (loading)
    return (
      <main className={styles.auth}>
        <Loading />
      </main>
    );
  if (error)
    return (
      <main className={styles.auth}>
        <ErrorMessage error={error} />
        <button
          onClick={() => {
            setError(undefined);
            setAttempt((x) => x + 1);
          }}
        >
          Retry connection
        </button>
      </main>
    );
  return (
    <AuthContext.Provider value={{ user, setUser }}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<AuthPage />} />
          <Route path="/register" element={<AuthPage register />} />
          <Route element={<Shell />}>
            <Route path="/tickets" element={<TicketList />} />
            <Route path="/tickets/new" element={<NewTicket />} />
            <Route path="/tickets/:id" element={<TicketDetail />} />
            <Route path="/categories" element={<Categories />} />
          </Route>
          <Route path="/" element={<Navigate to="/tickets" replace />} />
          <Route
            path="*"
            element={
              <main className={styles.auth}>
                <h1>Page not found</h1>
                <Link to="/tickets">Back to tickets</Link>
              </main>
            }
          />
        </Routes>
      </BrowserRouter>
    </AuthContext.Provider>
  );
}
