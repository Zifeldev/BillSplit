import * as SecureStore from 'expo-secure-store';
import { Platform } from 'react-native';

/**
 * Persistent key/value storage: expo-secure-store on native, localStorage on web.
 * Every access is wrapped in try/catch — storage can be unavailable (private
 * browsing, SSR, keychain errors) and the app must keep working without it.
 */

const isWeb = Platform.OS === 'web';

function webStorage(): Storage | null {
  try {
    if (typeof window !== 'undefined' && window.localStorage) return window.localStorage;
  } catch {
    // ignore
  }
  return null;
}

export async function getItem(key: string): Promise<string | null> {
  try {
    if (isWeb) return webStorage()?.getItem(key) ?? null;
    return await SecureStore.getItemAsync(key);
  } catch {
    return null;
  }
}

export async function setItem(key: string, value: string): Promise<void> {
  try {
    if (isWeb) {
      webStorage()?.setItem(key, value);
      return;
    }
    await SecureStore.setItemAsync(key, value);
  } catch {
    // ignore
  }
}

export async function removeItem(key: string): Promise<void> {
  try {
    if (isWeb) {
      webStorage()?.removeItem(key);
      return;
    }
    await SecureStore.deleteItemAsync(key);
  } catch {
    // ignore
  }
}

const ACCESS = 'billsplit.accessToken';
const REFRESH = 'billsplit.refreshToken';
const PENDING_INVITE = 'billsplit.pendingInvite';

export interface Tokens {
  accessToken: string;
  refreshToken: string;
}

let memoryTokens: Tokens | null = null;

export async function loadTokens(): Promise<Tokens | null> {
  const [accessToken, refreshToken] = await Promise.all([getItem(ACCESS), getItem(REFRESH)]);
  memoryTokens = accessToken && refreshToken ? { accessToken, refreshToken } : null;
  return memoryTokens;
}

export function currentTokens(): Tokens | null {
  return memoryTokens;
}

export async function saveTokens(tokens: Tokens): Promise<void> {
  memoryTokens = tokens;
  await Promise.all([setItem(ACCESS, tokens.accessToken), setItem(REFRESH, tokens.refreshToken)]);
}

export async function clearTokens(): Promise<void> {
  memoryTokens = null;
  await Promise.all([removeItem(ACCESS), removeItem(REFRESH)]);
}

export const pendingInvite = {
  get: () => getItem(PENDING_INVITE),
  set: (token: string) => setItem(PENDING_INVITE, token),
  clear: () => removeItem(PENDING_INVITE),
};

const PUSH_TOKEN = 'billsplit.pushToken';

/** The Expo push token registered with the backend for this session (to unregister on logout). */
export const registeredPushToken = {
  get: () => getItem(PUSH_TOKEN),
  set: (token: string) => setItem(PUSH_TOKEN, token),
  clear: () => removeItem(PUSH_TOKEN),
};
