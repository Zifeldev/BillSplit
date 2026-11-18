import { Icon } from '@/components/Icon';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Stack, useIsFocused, useRouter } from 'expo-router';
import React, { useEffect, useRef, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useNotifications } from '@/api/queries';
import { NotificationApi } from '@/api/services';
import type { AppNotification, ListNotificationsResponse } from '@/api/types';
import { Banner, Button, Card, EmptyView, ErrorView, IconName, ListRow, Screen, SkeletonList, Txt } from '@/components/ui';
import { notificationHref } from '@/lib/notification-route';
import { radius, useTheme } from '@/theme';

const ICONS: Record<string, IconName> = {
  friend_request: 'person-add-outline',
  friend_accepted: 'people-outline',
  group_joined: 'people-circle-outline',
  receipt_parsed: 'receipt-outline',
  receipt_finalized: 'lock-closed-outline',
  expense_added: 'cash-outline',
  settlement: 'checkmark-done-outline',
  recurring_upcoming: 'repeat-outline',
};

function timeAgo(ts: string | null): string {
  if (!ts) return '';
  const d = new Date(ts).getTime();
  if (Number.isNaN(d)) return '';
  const s = Math.max(0, (Date.now() - d) / 1000);
  if (s < 60) return 'just now';
  if (s < 3600) return `${Math.floor(s / 60)} min ago`;
  if (s < 86400) return `${Math.floor(s / 3600)} h ago`;
  return new Date(ts).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

function markLocal(data: ListNotificationsResponse | undefined, ids: string[] | null): ListNotificationsResponse | undefined {
  if (!data) return data;
  const all = ids === null;
  const set = new Set(ids ?? []);
  const notifications = data.notifications.map((n) => (all || set.has(n.id) ? { ...n, read: true } : n));
  return { notifications, unreadCount: notifications.filter((n) => !n.read).length };
}

export default function NotificationsScreen() {
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const focused = useIsFocused();
  const q = useNotifications(focused);
  const [error, setError] = useState<string | null>(null);
  // Rows that were unread when the inbox was opened stay highlighted for this visit.
  const [fresh, setFresh] = useState<Set<string> | null>(null);
  if (fresh === null && q.data) {
    setFresh(new Set(q.data.notifications.filter((n) => !n.read).map((n) => n.id)));
  }
  const markedOnOpen = useRef(false);

  const markRead = useMutation({
    mutationFn: (ids: string[] | null) => NotificationApi.markRead({ ids: ids ?? [] }),
    onMutate: (ids) => {
      qc.setQueryData<ListNotificationsResponse>(qk.notifications, (prev) => markLocal(prev, ids));
    },
    onError: (e) => {
      setError(errorMessage(e));
      void qc.invalidateQueries({ queryKey: qk.notifications });
    },
  });

  // Mark everything visible as read on open.
  const { mutate } = markRead;
  useEffect(() => {
    if (!fresh || markedOnOpen.current) return;
    markedOnOpen.current = true;
    if (fresh.size > 0) mutate([...fresh]);
  }, [fresh, mutate]);

  const open = (n: AppNotification) => {
    if (!n.read) markRead.mutate([n.id]);
    router.push(notificationHref(n.data));
  };

  const list = q.data?.notifications ?? [];

  return (
    <Screen refreshing={q.isRefetching} onRefresh={() => void q.refetch()}>
      <Stack.Screen options={{ title: 'Notifications' }} />
      {error ? <Banner kind="error" message={error} /> : null}
      {q.data ? (
        list.length === 0 ? (
          <Card>
            <EmptyView icon="notifications" title="No notifications" message="Friend requests, new receipts and settlements show up here." />
          </Card>
        ) : (
          <>
            {q.data.unreadCount > 0 ? (
              <Button title="Mark all as read" small variant="ghost" onPress={() => markRead.mutate(null)} />
            ) : null}
            <Card>
              {list.map((n) => {
                const highlight = !n.read || !!fresh?.has(n.id);
                return (
                  <ListRow
                    key={n.id}
                    onPress={() => open(n)}
                    left={
                      <View style={[styles.icon, { backgroundColor: highlight ? c.primarySoft : c.cardAlt }]}>
                        <Icon name={ICONS[n.type] ?? 'notifications-outline'} size={18} color={highlight ? c.primary : c.textMuted} />
                      </View>
                    }
                    title={
                      <Txt weight={highlight ? '800' : '600'} numberOfLines={1}>
                        {n.title || 'Notification'}
                      </Txt>
                    }
                    subtitle={
                      <>
                        {n.body ? (
                          <Txt variant="muted" numberOfLines={3}>
                            {n.body}
                          </Txt>
                        ) : null}
                        <Txt variant="small">{timeAgo(n.createdAt)}</Txt>
                      </>
                    }
                    right={highlight ? <View style={[styles.dot, { backgroundColor: c.primary }]} /> : undefined}
                  />
                );
              })}
            </Card>
          </>
        )
      ) : q.error ? (
        <ErrorView error={q.error} onRetry={() => void q.refetch()} />
      ) : (
        <SkeletonList rows={4} />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  icon: { width: 38, height: 38, borderRadius: 19, alignItems: 'center', justifyContent: 'center' },
  dot: { width: 9, height: 9, borderRadius: radius.pill },
});
