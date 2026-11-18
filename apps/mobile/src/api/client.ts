import { Platform } from 'react-native';

import { clearTokens, currentTokens, saveTokens } from './tokens';
import type { ApiErrorBody, AuthResponse, RefreshRequest } from './types';

function defaultBaseUrl(): string {
  // Android emulator reaches the host machine through 10.0.2.2.
  return Platform.OS === 'android' ? 'http://10.0.2.2:8088' : 'http://localhost:8088';
}

export const API_URL: string = (process.env.EXPO_PUBLIC_API_URL || defaultBaseUrl()).replace(
  /\/+$/,
  '',
);

export class ApiError extends Error {
  readonly status: number;
  readonly code: number;

  constructor(status: number, code: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }

  get isRateLimited(): boolean {
    return this.status === 429;
  }
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.isRateLimited) return err.message || 'Too many requests — please slow down and try again.';
    return err.message || `Request failed (${err.status})`;
  }
  if (err instanceof Error) {
    if (err.message === 'Failed to fetch' || err.message.includes('Network request failed')) {
      return `Cannot reach the server at ${API_URL}`;
    }
    return err.message;
  }
  return 'Something went wrong';
}

// ---------- JSON normalization ----------
type Json = null | boolean | number | string | Json[] | { [key: string]: Json };

function snakeToCamel(key: string): string {
  return key.includes('_') ? key.replace(/_([a-z0-9])/gi, (_m, c: string) => c.toUpperCase()) : key;
}

/** Fields whose value is a proto `map<string,string>`: keys are user data, keep them verbatim. */
const MAP_FIELDS = new Set(['data']);

/**
 * - Converts snake_case keys to lowerCamelCase (defensive, in case the gateway
 *   is configured with UseProtoNames). Keys inside map fields are left as-is.
 * - Converts every `...Cents` value (int64 → JSON string) to a number.
 */
function normalize(value: Json, keepKeys = false): Json {
  if (Array.isArray(value)) return value.map((v) => normalize(v));
  if (value !== null && typeof value === 'object') {
    const out: { [key: string]: Json } = {};
    for (const [rawKey, rawVal] of Object.entries(value)) {
      if (keepKeys) {
        out[rawKey] = rawVal;
        continue;
      }
      const key = snakeToCamel(rawKey);
      if (key.endsWith('Cents') && (typeof rawVal === 'string' || typeof rawVal === 'number')) {
        const n = Number(rawVal);
        out[key] = Number.isFinite(n) ? n : 0;
      } else {
        out[key] = normalize(rawVal, MAP_FIELDS.has(key));
      }
    }
    return out;
  }
  return value;
}

function isErrorBody(v: unknown): v is ApiErrorBody {
  return typeof v === 'object' && v !== null && 'message' in v;
}

// ---------- auth hooks ----------
type AuthFailureListener = () => void;
let onAuthFailure: AuthFailureListener | null = null;
export function setAuthFailureListener(fn: AuthFailureListener | null): void {
  onAuthFailure = fn;
}

type RefreshResult = 'ok' | 'invalid' | 'error';
let refreshInFlight: Promise<RefreshResult> | null = null;

/**
 * Refresh tokens are single-use and rotate: every successful Refresh returns a
 * new refresh token that must replace the old one. Concurrent 401s share one
 * in-flight refresh so the (single-use) token is only spent once.
 */
function refreshTokens(): Promise<RefreshResult> {
  if (refreshInFlight) return refreshInFlight;
  const tokens = currentTokens();
  if (!tokens) return Promise.resolve('invalid');
  refreshInFlight = (async (): Promise<RefreshResult> => {
    try {
      const body: RefreshRequest = { refreshToken: tokens.refreshToken };
      const res = await rawCall<AuthResponse>('AuthService', 'Refresh', body, null);
      if (!res.accessToken || !res.refreshToken) return 'invalid';
      await saveTokens({ accessToken: res.accessToken, refreshToken: res.refreshToken });
      return 'ok';
    } catch (err) {
      // 4xx — the refresh token is expired/revoked/already used; anything else is transient.
      return err instanceof ApiError && err.status >= 400 && err.status < 500 ? 'invalid' : 'error';
    } finally {
      refreshInFlight = null;
    }
  })();
  return refreshInFlight;
}

async function rawCall<Res>(
  service: string,
  method: string,
  body: object,
  accessToken: string | null,
): Promise<Res> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'application/json',
  };
  if (accessToken) headers.Authorization = `Bearer ${accessToken}`;

  const res = await fetch(`${API_URL}/billsplit.v1.${service}/${method}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });

  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = null;
    }
  }

  if (!res.ok) {
    const msg = isErrorBody(parsed)
      ? String(parsed.message)
      : res.status === 429
        ? 'Too many requests — please try again in a moment.'
        : text || res.statusText || `HTTP ${res.status}`;
    const code = isErrorBody(parsed) && typeof parsed.code === 'number' ? parsed.code : 0;
    throw new ApiError(res.status, code, msg);
  }
  return normalize((parsed ?? {}) as Json) as unknown as Res;
}

/** Never send a token. */
const PUBLIC_METHODS = new Set(['AuthService/Register', 'AuthService/Login', 'AuthService/Refresh']);
/** Work without login; the token (if any) is still sent so the server can personalize. */
const OPTIONAL_AUTH_METHODS = new Set(['FinanceService/ListCurrencies', 'GroupService/GetInvite']);
/** Sent with the current token if any, but never triggers refresh/logout. */
const NO_REFRESH_METHODS = new Set(['AuthService/Logout', 'NotificationService/UnregisterDevice']);

async function handleAuthFailure(): Promise<void> {
  await clearTokens();
  onAuthFailure?.();
}

/**
 * Calls `POST {base}/billsplit.v1.<service>/<method>`.
 * On 401 for an authenticated call, refreshes tokens (single in-flight refresh)
 * and retries once; if the refresh token is rejected, clears tokens and notifies
 * the auth layer (logout).
 */
export async function rpc<Req extends object, Res>(
  service: string,
  method: string,
  body: Req,
): Promise<Res> {
  const name = `${service}/${method}`;
  if (PUBLIC_METHODS.has(name)) return rawCall<Res>(service, method, body, null);

  const used = currentTokens()?.accessToken ?? null;
  if (!used && OPTIONAL_AUTH_METHODS.has(name)) return rawCall<Res>(service, method, body, null);
  if (NO_REFRESH_METHODS.has(name)) return rawCall<Res>(service, method, body, used);

  try {
    return await rawCall<Res>(service, method, body, used);
  } catch (err) {
    if (!(err instanceof ApiError) || err.status !== 401) throw err;

    // Another request may already have rotated the tokens — then just retry.
    const now = currentTokens()?.accessToken ?? null;
    const result: RefreshResult = now && now !== used ? 'ok' : await refreshTokens();
    if (result === 'error') throw err;
    if (result === 'invalid') {
      await handleAuthFailure();
      // Public-capable methods still work anonymously.
      if (OPTIONAL_AUTH_METHODS.has(name)) return rawCall<Res>(service, method, body, null);
      throw err;
    }
    try {
      return await rawCall<Res>(service, method, body, currentTokens()?.accessToken ?? null);
    } catch (retryErr) {
      if (retryErr instanceof ApiError && retryErr.status === 401) await handleAuthFailure();
      throw retryErr;
    }
  }
}
