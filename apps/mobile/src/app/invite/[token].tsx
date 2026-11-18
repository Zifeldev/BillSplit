import { Icon } from '@/components/Icon';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk } from '@/api/queries';
import { FriendApi, GroupApi } from '@/api/services';
import { pendingInvite } from '@/api/tokens';
import { useAuth } from '@/auth/AuthContext';
import { Banner, Button, Card, ErrorView, LoadingView, Screen, Txt } from '@/components/ui';
import { spacing, useTheme } from '@/theme';

function InviteBody({ token, authed }: { token: string; authed: boolean }) {
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const [error, setError] = useState<string | null>(null);

  // Guests: remember the invite so it reopens after login/registration.
  useEffect(() => {
    if (authed) void pendingInvite.clear();
    else void pendingInvite.set(token);
  }, [authed, token]);

  // GetInvite is public; alreadyMember is only meaningful when signed in.
  const preview = useQuery({
    queryKey: [...qk.invite(token), authed],
    queryFn: () => GroupApi.getInvite({ token }),
  });
  const p = preview.data;
  const isGroup = p?.kind === 'INVITE_KIND_GROUP';

  const accept = useMutation({
    mutationFn: async (): Promise<string> => {
      if (isGroup) {
        const g = await GroupApi.join({ token });
        return `/group/${g.id}`;
      }
      await FriendApi.acceptInvite({ token });
      return '/friends';
    },
    onSuccess: (dest) => {
      void qc.invalidateQueries({ queryKey: qk.groups });
      void qc.invalidateQueries({ queryKey: qk.friends });
      void qc.invalidateQueries({ queryKey: qk.friendRequests });
      void qc.invalidateQueries({ queryKey: qk.myBalances });
      router.replace(dest);
    },
    onError: (e) => setError(errorMessage(e)),
  });

  if (!p) {
    return (
      <Screen>
        {preview.error ? (
          <ErrorView error={preview.error} onRetry={() => void preview.refetch()} />
        ) : (
          <LoadingView label="Loading invitation…" />
        )}
        {preview.error ? (
          <Button title={authed ? 'Go home' : 'Log in'} variant="ghost" onPress={() => router.replace(authed ? '/' : '/login')} />
        ) : null}
      </Screen>
    );
  }

  return (
    <Screen>
      {error ? <Banner kind="error" message={error} /> : null}
      <Card style={styles.hero}>
        <View style={[styles.iconWrap, { backgroundColor: c.primarySoft }]}>
          <Icon name={isGroup ? 'people' : 'person-add'} size={32} color={c.primary} />
        </View>
        {isGroup ? (
          <>
            <Txt variant="muted">{p.inviterName || 'Someone'} invited you to join</Txt>
            <Txt variant="title" style={styles.center}>
              {p.groupName || 'a group'}
            </Txt>
            <Txt variant="muted">
              {p.membersCount} {p.membersCount === 1 ? 'member' : 'members'}
            </Txt>
          </>
        ) : (
          <>
            <Txt variant="title" style={styles.center}>
              {p.inviterName || 'Someone'}
            </Txt>
            <Txt variant="muted">wants to be your friend on Billsplit</Txt>
          </>
        )}
      </Card>
      {p.expired ? <Banner kind="error" title="Invitation expired" message="Ask for a new link." /> : null}
      {!authed ? (
        p.expired ? null : (
          <>
            <Txt variant="muted" style={styles.center}>
              Log in or create an account to {isGroup ? 'join the group' : 'accept'} — we&apos;ll bring you right back here.
            </Txt>
            <Button title="Log in" icon="log-in-outline" onPress={() => router.push('/login')} />
            <Button title="Create account" variant="secondary" onPress={() => router.push('/register')} />
          </>
        )
      ) : p.alreadyMember ? (
        <>
          <Banner kind="info" message={isGroup ? 'You are already a member of this group.' : 'You are already friends.'} />
          <Button
            title={isGroup ? 'Open group' : 'Open friends'}
            onPress={() => router.replace(isGroup && p.groupId ? `/group/${p.groupId}` : '/friends')}
          />
        </>
      ) : !p.expired ? (
        <Button
          title={isGroup ? 'Join group' : 'Add friend'}
          icon="checkmark"
          loading={accept.isPending}
          onPress={() => {
            setError(null);
            accept.mutate();
          }}
        />
      ) : null}
      {authed ? <Button title="Not now" variant="ghost" onPress={() => router.replace('/')} /> : null}
    </Screen>
  );
}

export default function InviteScreen() {
  const { token } = useLocalSearchParams<{ token: string }>();
  const { status } = useAuth();
  const t = String(token ?? '');
  if (!t) return <ErrorView error={new Error('Invalid invitation link')} />;
  return <InviteBody token={t} authed={status === 'authed'} />;
}

const styles = StyleSheet.create({
  hero: { alignItems: 'center', paddingVertical: spacing.xl, gap: spacing.sm },
  iconWrap: { width: 64, height: 64, borderRadius: 32, alignItems: 'center', justifyContent: 'center' },
  center: { textAlign: 'center' },
});
