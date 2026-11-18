/**
 * Live receipt updates over WebSocket (gateway: /v1/realtime/receipts/{id}).
 *
 * Events only say "something changed": the screen refetches the receipt and
 * split, so the server stays the source of truth and a missed event costs at
 * most one stale render. Presence tells who else has the receipt open.
 */
import { useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { AppState } from 'react-native';

import { API_URL } from '@/api/client';
import { qk } from '@/api/queries';
import { currentTokens } from '@/api/tokens';

export interface LiveEvent {
  type: 'hello' | 'presence' | 'receipt.updated' | 'receipt.deleted';
  receipt_id?: string;
  action?: 'claim' | 'items' | 'parsed' | 'finalized';
  user_id?: string;
  item_ids?: string[];
  /** Claimant's new shares per item (0 — dropped). */
  shares?: number[];
  viewers?: string[];
}

const WS_URL = API_URL.replace(/^http/, 'ws');
const MAX_BACKOFF = 15000;

export function useReceiptLive(
  receiptId: string,
  myId: string,
  onEvent?: (ev: LiveEvent) => void,
): { connected: boolean; viewers: string[] } {
  const qc = useQueryClient();
  const [connected, setConnected] = useState(false);
  const [viewers, setViewers] = useState<string[]>([]);
  const onEventRef = useRef(onEvent);
  useEffect(() => {
    onEventRef.current = onEvent;
  });

  useEffect(() => {
    if (!receiptId || !myId) return;
    let ws: WebSocket | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let backoff = 1000;
    let stopped = false;
    let everOpened = false;

    const refetch = () =>
      Promise.all([
        qc.invalidateQueries({ queryKey: qk.receipt(receiptId) }),
        qc.invalidateQueries({ queryKey: qk.split(receiptId) }),
      ]);

    const connect = () => {
      if (stopped) return;
      const token = currentTokens()?.accessToken;
      if (!token) return;
      ws = new WebSocket(`${WS_URL}/v1/realtime/receipts/${receiptId}?access_token=${encodeURIComponent(token)}`);
      ws.onopen = () => {
        backoff = 1000;
        setConnected(true);
        // Catch up on anything that changed while we were offline.
        if (everOpened) void refetch();
        everOpened = true;
      };
      ws.onmessage = (m) => {
        let ev: LiveEvent;
        try {
          ev = JSON.parse(String(m.data)) as LiveEvent;
        } catch {
          return;
        }
        if (ev.type === 'presence') {
          setViewers(ev.viewers ?? []);
        } else if (ev.type === 'receipt.updated' || ev.type === 'receipt.deleted') {
          // Our own claims are already applied optimistically.
          if (ev.user_id !== myId || ev.action !== 'claim') void refetch();
          if (ev.action === 'finalized') {
            void qc.invalidateQueries({ queryKey: qk.myBalances });
            void qc.invalidateQueries({ queryKey: qk.dashboardAll });
          }
        }
        onEventRef.current?.(ev);
      };
      ws.onclose = () => {
        setConnected(false);
        ws = null;
        if (stopped) return;
        // The handshake fails on an expired token: a regular API call refreshes it.
        timer = setTimeout(() => {
          void refetch().finally(connect);
        }, backoff);
        backoff = Math.min(backoff * 2, MAX_BACKOFF);
      };
    };

    connect();
    // Mobile OSes drop sockets in the background; reconnect right away on return.
    const sub = AppState.addEventListener('change', (st) => {
      if (st === 'active' && !ws && !stopped) {
        if (timer) clearTimeout(timer);
        backoff = 1000;
        void refetch().finally(connect);
      }
    });
    return () => {
      stopped = true;
      sub.remove();
      if (timer) clearTimeout(timer);
      ws?.close();
      setConnected(false);
      setViewers([]);
    };
  }, [receiptId, myId, qc]);

  return { connected, viewers };
}
