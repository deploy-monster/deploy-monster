const API_BASE = '/api/v1';

interface RequestOptions {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
}

class APIError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'APIError';
    this.status = status;
  }
}

function getCSRFToken(): string {
  const match = document.cookie.match(/(?:^|;\s*)dm_csrf=([^;]*)/);
  return match ? match[1] : '';
}

async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, headers = {} } = options;

  // Add CSRF token for mutating requests
  if (method !== 'GET' && method !== 'HEAD') {
    const csrf = getCSRFToken();
    if (csrf) {
      headers['X-CSRF-Token'] = csrf;
    }
  }

  const config: RequestInit = {
    method,
    credentials: 'include', // send httpOnly cookies
    headers: {
      'Content-Type': 'application/json',
      ...headers,
    },
  };

  if (body) {
    config.body = JSON.stringify(body);
  }

  const response = await fetch(`${API_BASE}${path}`, config);

  // Handle 401 - try refresh
  if (response.status === 401) {
    const refreshed = await tryRefresh();
    if (refreshed) {
      return request<T>(path, options);
    }
    window.location.href = '/login';
    throw new APIError(401, 'Session expired');
  }

  if (!response.ok) {
    const data = await response.json().catch(() => ({ error: response.statusText }));
    throw new APIError(response.status, data.error || response.statusText);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const json = await response.json();

  // Unwrap {data: ...} responses for array types
  if (json && typeof json === 'object' && 'data' in json && !('error' in json)) {
    return json.data as T;
  }

  return json as T;
}

async function tryRefresh(): Promise<boolean> {
  try {
    const response = await fetch(`${API_BASE}/auth/refresh`, {
      method: 'POST',
      credentials: 'include', // sends dm_refresh cookie
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });

    if (!response.ok) return false;
    // Cookies are set by the server — nothing to store client-side
    return true;
  } catch {
    return false;
  }
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body }),
  put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body }),
  patch: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PATCH', body }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
};

export { APIError };
