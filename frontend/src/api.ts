export type User = {
  id: number;
  name: string;
  email: string;
  role: "user" | "operator";
};
export type Category = { id: number; name: string; description: string };
export type Ticket = {
  id: number;
  requester_id: number;
  requester_name: string;
  assignee_name: string;
  assignee_id: number | null;
  category_id: number;
  title: string;
  description: string;
  priority: string;
  status: string;
  created_at: string;
  updated_at: string;
};
export type Comment = {
  id: number;
  author_id: number;
  author_name: string;
  author_role: string;
  body: string;
  created_at: string;
  updated_at: string;
};
export type Page<T> = {
  items: T[];
  total: number;
  page: number;
  page_size: number;
};
export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch("/api/v1" + path, {
    method,
    credentials: "same-origin",
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!response.ok) {
    const data = await response.json().catch(() => null);
    if (response.status === 401 && !path.startsWith("/auth/"))
      window.dispatchEvent(new Event("session-expired"));
    throw new APIError(
      response.status,
      data?.error?.message || "Unable to reach the server",
    );
  }
  return response.status === 204 ? (undefined as T) : response.json();
}
