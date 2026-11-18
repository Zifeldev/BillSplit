import { Icon } from '@/components/Icon';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useFriendRequests, useFriends, useMyBalances } from '@/api/queries';
import { BalanceApi, FriendApi, GroupApi } from '@/api/services';
import type { FriendBalance, InviteLink, Transfer } from '@/api/types';
import { useMyId } from '@/auth/AuthContext';
import { Celebration } from '@/components/Celebration';
import { PayButtons } from '@/components/PayButtons';
import {
  Avatar,
  Badge,
  Banner,
  Button,
  Card,
  EmptyView,
  ErrorView,
  FadeIn,
  Field,
  IconButton,
  IconTile,
  ListRow,
  Money,
  Row,
  Screen,
  SectionHeader,
  SkeletonList,
  Txt,
} from '@/components/ui';
import { confirm, notify } from '@/lib/dialogs';
import { formatMoney } from '@/lib/format';
import { personHref } from '@/lib/person';
import { copyText, shareLink } from '@/lib/share';
import { radius, spacing, useTheme } from '@/theme';

export default function FriendsScreen() {
  const qc = useQueryClient();
  const router = useRouter();
  const { c } = useTheme();
  const friends = useFriends();
  const requests = useFriendRequests();
  const balances = useMyBalances();
  const myId = useMyId();

  const [email, setEmail] = useState('');
  const [invite, setInvite] = useState<InviteLink | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);
  const [celebrate, setCelebrate] = useState<string | null>(null);

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: qk.friends });
    void qc.invalidateQueries({ queryKey: qk.friendRequests });
  };

  const send = useMutation({
    mutationFn: () => FriendApi.send({ email: email.trim() }),
    onSuccess: (f) => {
      setEmail('');
      setError(null);
      setInfo(f.status === 'FRIEND_STATUS_ACCEPTED' ? `You are now friends with ${f.name || f.email}` : `Request sent to ${f.email || 'user'}`);
      invalidate();
    },
    onError: (e) => {
      setInfo(null);
      setError(errorMessage(e));
    },
  });

  const respond = useMutation({
    mutationFn: (v: { userId: string; accept: boolean }) => FriendApi.respond(v),
    onSuccess: () => {
      invalidate();
      void qc.invalidateQueries({ queryKey: qk.myBalances });
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const remove = useMutation({
    mutationFn: (userId: string) => FriendApi.remove({ userId }),
    onSuccess: invalidate,
    onError: (e) => setError(errorMessage(e)),
  });

  /**
   * Payments to a friend are recorded per group (SettleUp needs a group): settle
   * every suggested transfer from me to this friend across shared groups.
   */
  const settleFriend = useMutation({
    mutationFn: async (friend: FriendBalance): Promise<number> => {
      const groups = (await GroupApi.listMine()).groups.filter((g) =>
        (g.members ?? []).some((m) => m.userId === friend.userId),
      );
      const todo: { groupId: string; groupName: string; currency: string; t: Transfer }[] = [];
      for (const g of groups) {
        const b = await BalanceApi.group({ groupId: g.id });
        for (const t of b.suggestedTransfers ?? []) {
          if (t.fromUserId === myId && t.toUserId === friend.userId && t.amountCents > 0) {
            todo.push({ groupId: g.id, groupName: g.name, currency: b.currency || g.currency, t });
          }
        }
      }
      if (todo.length === 0) return 0;
      const summary = todo.map((x) => `• ${x.groupName}: ${formatMoney(x.t.amountCents, x.currency)}`).join('\n');
      const ok = await confirm('Record payment', `Mark these debts to ${friend.name || 'your friend'} as paid?\n\n${summary}`, 'Mark as paid');
      if (!ok) return -1;
      for (const x of todo) {
        await BalanceApi.settleUp({ groupId: x.groupId, fromUserId: myId, toUserId: friend.userId, amountCents: x.t.amountCents });
      }
      return todo.length;
    },
    onSuccess: (n, friend) => {
      if (n === 0) {
        notify(
          'Nothing to record automatically',
          `No direct transfer to ${friend.name || 'this friend'} is suggested in your groups. Record the payment from a group's balances.`,
        );
        return;
      }
      if (n < 0) return;
      setCelebrate(`Payment to ${friend.name || 'your friend'} recorded. You're square!`);
      void qc.invalidateQueries({ queryKey: qk.myBalances });
      void qc.invalidateQueries({ queryKey: ['groupBalances'] });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const createInvite = useMutation({
    mutationFn: FriendApi.createInvite,
    onSuccess: (link) => setInvite(link),
    onError: (e) => setError(errorMessage(e)),
  });

  const balanceById = new Map((balances.data?.friends ?? []).map((b) => [b.userId, b]));
  const cur = balances.data?.currency || 'USD';
  const accepted = (friends.data?.friends ?? []).filter((f) => f.status !== 'FRIEND_STATUS_PENDING_INCOMING' && f.status !== 'FRIEND_STATUS_PENDING_OUTGOING');
  const friendIds = new Set(accepted.map((f) => f.userId));
  const otherBalances = (balances.data?.friends ?? []).filter((b) => !friendIds.has(b.userId) && b.netCents !== 0);
  const incoming = requests.data?.incoming ?? [];
  const outgoing = requests.data?.outgoing ?? [];

  const refreshing = friends.isRefetching || requests.isRefetching || balances.isRefetching;

  return (
    <Screen
      refreshing={refreshing}
      onRefresh={() => {
        void friends.refetch();
        void requests.refetch();
        void balances.refetch();
      }}>
      {error ? <Banner kind="error" message={error} /> : null}
      {info ? <Banner kind="success" message={info} /> : null}

      <Card>
        <Row gap={spacing.md}>
          <IconTile icon="person-add" color={c.primary} size={44} />
          <View style={styles.flex1}>
            <Txt variant="h3">Add a friend</Txt>
            <Txt variant="small">Split with them in any group</Txt>
          </View>
        </Row>
        <Row gap={spacing.sm}>
          <Field
            style={styles.flex1}
            placeholder="friend@example.com"
            value={email}
            onChangeText={setEmail}
            autoCapitalize="none"
            keyboardType="email-address"
            onSubmitEditing={() => email.trim() && send.mutate()}
          />
          <Button title="Send" icon="send" disabled={!email.trim()} loading={send.isPending} onPress={() => send.mutate()} />
        </Row>
        <Txt variant="small">Not on Billsplit yet? Share your personal invite link — whoever opens it becomes your friend.</Txt>
        {invite ? (
          <>
            <View style={[styles.linkBox, { backgroundColor: c.cardAlt }]}>
              <Txt variant="muted" color={c.text} numberOfLines={2}>
                {invite.url || invite.appUrl}
              </Txt>
            </View>
            <Row gap={spacing.sm}>
              <Button
                title="Share"
                icon="share-outline"
                small
                style={styles.flex1}
                onPress={() => void shareLink(invite.url || invite.appUrl, 'Add me on Billsplit')}
              />
              <Button
                title="Copy"
                icon="copy-outline"
                small
                variant="secondary"
                style={styles.flex1}
                onPress={() => void copyText(invite.url || invite.appUrl)}
              />
            </Row>
          </>
        ) : (
          <Button
            title="Share my invite link"
            icon="link-outline"
            variant="secondary"
            loading={createInvite.isPending}
            onPress={() => createInvite.mutate()}
          />
        )}
      </Card>

      {incoming.length > 0 ? (
        <>
          <SectionHeader title={`Requests (${incoming.length})`} />
          <Card>
            {incoming.map((f) => (
              <ListRow
                key={f.userId}
                left={<Avatar name={f.name} id={f.userId} />}
                title={f.name || f.email}
                subtitle={f.email}
                right={
                  <Row gap={6}>
                    <Button
                      title="Accept"
                      small
                      loading={respond.isPending && respond.variables?.userId === f.userId && respond.variables.accept}
                      onPress={() => respond.mutate({ userId: f.userId, accept: true })}
                    />
                    <Button
                      title="Decline"
                      small
                      variant="ghost"
                      onPress={() => respond.mutate({ userId: f.userId, accept: false })}
                    />
                  </Row>
                }
              />
            ))}
          </Card>
        </>
      ) : null}

      {outgoing.length > 0 ? (
        <>
          <SectionHeader title="Sent requests" />
          <Card>
            {outgoing.map((f) => (
              <ListRow
                key={f.userId}
                left={<Avatar name={f.name} id={f.userId} />}
                title={f.name || f.email}
                subtitle="Waiting for response"
                right={
                  <IconButton
                    icon="close-circle-outline"
                    color={c.textMuted}
                    label="Cancel request"
                    onPress={() => remove.mutate(f.userId)}
                  />
                }
              />
            ))}
          </Card>
        </>
      ) : null}

      <SectionHeader title="Friends" />
      {friends.data ? (
        accepted.length === 0 ? (
          <Card>
            <EmptyView
              icon="people"
              title="No friends yet"
              message="Add friends by email or share your invite link — splitting is more fun together."
              action={
                <Button title="Share my invite link" icon="link" small loading={createInvite.isPending} onPress={() => createInvite.mutate()} />
              }
            />
          </Card>
        ) : (
          <Card>
            {accepted.map((f, i) => {
              const b = balanceById.get(f.userId);
              const net = b?.netCents ?? 0;
              return (
                <FadeIn key={f.userId} index={i}>
                  <ListRow
                    left={<Avatar name={f.name} id={f.userId} />}
                    divider={i > 0}
                    title={f.name || f.email}
                    subtitle={
                      <Row gap={6}>
                        <Badge
                          label={net > 0 ? 'owes you' : net < 0 ? 'you owe' : 'settled up'}
                          tone={net > 0 ? 'ok' : net < 0 ? 'bad' : 'muted'}
                          icon={net > 0 ? 'arrow-down' : net < 0 ? 'arrow-up' : 'checkmark'}
                        />
                      </Row>
                    }
                    onPress={() => router.push(personHref(f.userId, f.name || f.email))}
                    onLongPress={async () => {
                      const ok = await confirm('Remove friend', `Remove ${f.name || f.email} from friends?`, 'Remove', true);
                      if (ok) remove.mutate(f.userId);
                    }}
                    right={
                      <Row gap={4}>
                        <Money cents={net} currency={cur} signed colored />
                        <Icon name="chevron-forward" size={16} color={c.textMuted} />
                      </Row>
                    }
                  />
                  {b && net < 0 ? (
                    <PayButtons
                      handles={b.payment ?? f.payment}
                      amountCents={-net}
                      currency={cur}
                      note="Billsplit"
                      recipientName={b.name || f.name || f.email}
                      onMarkPaid={() => settleFriend.mutate({ ...b, name: b.name || f.name })}
                    />
                  ) : null}
                </FadeIn>
              );
            })}
            <Txt variant="small">Tap a friend for your shared account · long-press to remove. Amounts in {cur}, across all groups.</Txt>
          </Card>
        )
      ) : friends.error ? (
        <ErrorView error={friends.error} onRetry={() => void friends.refetch()} />
      ) : (
        <SkeletonList rows={3} />
      )}

      {otherBalances.length > 0 ? (
        <>
          <SectionHeader title="Other group members" />
          <Card>
            {otherBalances.map((b) => (
              <View key={b.userId}>
                <ListRow
                  left={<Avatar name={b.name} id={b.userId} />}
                  title={b.name || 'Group member'}
                  onPress={() => router.push(personHref(b.userId, b.name))}
                  subtitle={
                    <Row gap={6}>
                      <Badge label={b.netCents > 0 ? 'owes you' : 'you owe'} tone={b.netCents > 0 ? 'ok' : 'bad'} />
                    </Row>
                  }
                  right={
                    <Row gap={4}>
                      <Money cents={b.netCents} currency={cur} signed colored />
                      <Icon name="chevron-forward" size={16} color={c.textMuted} />
                    </Row>
                  }
                />
                {b.netCents < 0 ? (
                  <PayButtons
                    handles={b.payment}
                    amountCents={-b.netCents}
                    currency={cur}
                    note="Billsplit"
                    recipientName={b.name || 'them'}
                    onMarkPaid={() => settleFriend.mutate(b)}
                  />
                ) : null}
              </View>
            ))}
          </Card>
        </>
      ) : null}
      <Celebration visible={!!celebrate} icon="handshake" title="Settled up!" message={celebrate ?? undefined} onClose={() => setCelebrate(null)} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  linkBox: { padding: spacing.md, borderRadius: radius.md },
});
