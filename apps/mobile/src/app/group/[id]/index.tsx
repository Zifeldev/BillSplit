import { Icon } from '@/components/Icon';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useFriends, useGroup, useGroupBalances, useGroupInvites, useReceipts } from '@/api/queries';
import { BalanceApi, GroupApi } from '@/api/services';
import type { Receipt, ReceiptStatus, Transfer } from '@/api/types';
import { statusLabel } from '@/lib/receipt';
import { useMyId } from '@/auth/AuthContext';
import { Celebration } from '@/components/Celebration';
import { PayButtons } from '@/components/PayButtons';
import {
  Avatar,
  AvatarStack,
  Badge,
  Banner,
  Button,
  Card,
  EmptyView,
  ErrorView,
  FadeIn,
  GlassPill,
  HeroCard,
  IconButton,
  IconName,
  IconTile,
  ListRow,
  Money,
  Row,
  Screen,
  SectionHeader,
  SelectModal,
  Skeleton,
  SkeletonList,
  Txt,
} from '@/components/ui';
import { frosted } from '@/components/Glass';
import { groupCover } from '@/lib/colors';
import { tapFeedback } from '@/lib/haptics';
import { confirm } from '@/lib/dialogs';
import { formatDate, formatMoney } from '@/lib/format';
import { personHref } from '@/lib/person';
import { copyText, shareLink } from '@/lib/share';
import { radius, softShadow, spacing, tone, useTheme } from '@/theme';

/** "You" for the current user, otherwise the server-provided name. */
function who(userId: string, name: string, myId: string): string {
  if (userId === myId) return 'You';
  return name || 'Unknown';
}

function StatusBadge({ status }: { status: ReceiptStatus }) {
  const s = statusLabel(status);
  return <Badge label={s.label} tone={s.tone} />;
}

function ActionTile({
  icon,
  label,
  onPress,
}: {
  icon: IconName;
  label: string;
  onPress: () => void;
}) {
  const { c } = useTheme();
  return (
    <Pressable
      accessibilityRole="button"
      onPress={() => {
        tapFeedback();
        onPress();
      }}
      style={({ pressed }) => [
        styles.action,
        frosted(c),
        softShadow(c),
        pressed && { transform: [{ scale: 0.96 }], opacity: 0.9 },
      ]}>
      <IconTile icon={icon} color={c.primary} size={44} />
      <Txt variant="small" weight="700" color={c.text} numberOfLines={1}>
        {label}
      </Txt>
    </Pressable>
  );
}

const WHITE = '#FFFFFF';
const WHITE_SOFT = 'rgba(255,255,255,0.85)';

export default function GroupDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const groupId = String(id ?? '');
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const myId = useMyId();

  const group = useGroup(groupId);
  const balances = useGroupBalances(groupId);
  const receipts = useReceipts(groupId);
  const friends = useFriends();

  const invites = useGroupInvites(groupId);
  const [addOpen, setAddOpen] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [celebrate, setCelebrate] = useState<string | null>(null);

  const g = group.data;
  const isOwner = !!g && g.ownerId === myId;

  const invalidateGroup = () => {
    void qc.invalidateQueries({ queryKey: qk.group(groupId) });
    void qc.invalidateQueries({ queryKey: qk.groups });
    void qc.invalidateQueries({ queryKey: qk.groupBalances(groupId) });
  };

  const createInvite = useMutation({
    mutationFn: () => GroupApi.createInvite({ groupId, ttlHours: 72, maxUses: 0 }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: qk.groupInvites(groupId) }),
    onError: (e) => setActionError(errorMessage(e)),
  });

  const revokeInvite = useMutation({
    mutationFn: (token: string) => GroupApi.revokeInvite({ token }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: qk.groupInvites(groupId) }),
    onError: (e) => setActionError(errorMessage(e)),
  });

  const addMember = useMutation({
    mutationFn: (userId: string) => GroupApi.addMember({ groupId, userId }),
    onSuccess: (updated) => {
      qc.setQueryData(qk.group(groupId), updated);
      invalidateGroup();
    },
    onError: (e) => setActionError(errorMessage(e)),
  });

  const removeMember = useMutation({
    mutationFn: (userId: string) => GroupApi.removeMember({ groupId, userId }),
    onSuccess: (_updated, userId) => {
      if (userId === myId) {
        void qc.invalidateQueries({ queryKey: qk.groups });
        void qc.invalidateQueries({ queryKey: qk.myBalances });
        qc.removeQueries({ queryKey: qk.group(groupId) });
        router.replace('/');
        return;
      }
      invalidateGroup();
    },
    onError: (e) => setActionError(errorMessage(e)),
  });

  const settle = useMutation({
    mutationFn: (t: Transfer) =>
      BalanceApi.settleUp({ groupId, fromUserId: t.fromUserId, toUserId: t.toUserId, amountCents: t.amountCents }),
    onSuccess: (_res, t) => {
      void qc.invalidateQueries({ queryKey: qk.groupBalances(groupId) });
      void qc.invalidateQueries({ queryKey: qk.myBalances });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
      setCelebrate(
        `${who(t.fromUserId, t.fromName, myId)} → ${who(t.toUserId, t.toName, myId)} · ${formatMoney(
          t.amountCents,
          balances.data?.currency || group.data?.currency || 'USD',
        )} recorded.`,
      );
    },
    onError: (e) => setActionError(errorMessage(e)),
  });

  const refreshing = group.isRefetching || balances.isRefetching || receipts.isRefetching;
  const onRefresh = () => {
    void group.refetch();
    void balances.refetch();
    void receipts.refetch();
    void invites.refetch();
  };

  if (!g) {
    return (
      <Screen>
        <Stack.Screen options={{ title: 'Group' }} />
        {group.error ? (
          <ErrorView error={group.error} onRetry={() => void group.refetch()} />
        ) : (
          <>
            <Skeleton height={180} r={radius.xl} />
            <SkeletonList rows={3} />
          </>
        )}
      </Screen>
    );
  }

  const memberIds = new Set((g.members ?? []).map((m) => m.userId));
  const addable = (friends.data?.friends ?? [])
    .filter((f) => f.status === 'FRIEND_STATUS_ACCEPTED' && !memberIds.has(f.userId))
    .map((f) => ({ value: f.userId, label: f.name || f.email, hint: f.email }));

  const currency = balances.data?.currency || g.currency;
  const cover = groupCover(g.id, g.name);
  const myNet = (balances.data?.balances ?? []).find((b) => b.userId === myId)?.netCents ?? 0;

  return (
    <Screen refreshing={refreshing} onRefresh={onRefresh}>
      <Stack.Screen options={{ title: g.name }} />
      {actionError ? <Banner kind="error" message={actionError} /> : null}

      <HeroCard color={cover.color}>
        <Row gap={spacing.md}>
          <View style={styles.coverEmoji}>
            <Icon name={cover.icon} size={30} color={WHITE} weight="duotone" />
          </View>
          <View style={styles.flex1}>
            <Txt variant="title" color={WHITE} numberOfLines={2}>
              {g.name}
            </Txt>
            <Txt variant="small" color={WHITE_SOFT} weight="600">
              {(g.members ?? []).length} members · {g.currency}
            </Txt>
          </View>
        </Row>
        <Row gap={spacing.sm} style={styles.heroRow}>
          <GlassPill>
            <Txt variant="small" color={WHITE_SOFT} weight="600">
              {myNet > 0 ? 'You are owed' : myNet < 0 ? 'You owe' : 'Your balance'}
            </Txt>
            {balances.data ? (
              <Money cents={Math.abs(myNet)} currency={currency} variant="big" color={WHITE} animate />
            ) : (
              <Skeleton width={120} height={30} style={styles.glassSkel} />
            )}
          </GlassPill>
          <View style={styles.heroAvatars}>
            <AvatarStack
              people={(g.members ?? []).map((m) => ({ id: m.userId, name: m.name || m.email }))}
              size={34}
              max={4}
              ring="rgba(255,255,255,0.9)"
            />
          </View>
        </Row>
      </HeroCard>

      <Row gap={spacing.sm}>
        <ActionTile
          icon="scan"
          label="Scan"
          onPress={() => router.push(`/group/${groupId}/add-receipt?mode=scan`)}
        />
        <ActionTile
          icon="create"
          label="Manual"
          onPress={() => router.push(`/group/${groupId}/add-receipt?mode=manual`)}
        />
        <ActionTile
          icon="cash"
          label="Expense"
          onPress={() => router.push(`/group/${groupId}/add-expense`)}
        />
      </Row>

      {/* Balances */}
      <SectionHeader title={`Balances · ${currency}`} />
      <Card>
        {balances.data ? (
          <>
            {(balances.data.balances ?? []).length === 0 ? (
              <Txt variant="muted">No expenses yet — balances appear once a receipt is finalized.</Txt>
            ) : (
              (balances.data.balances ?? []).map((b) => (
                <Pressable
                  key={b.userId}
                  disabled={b.userId === myId}
                  accessibilityRole={b.userId === myId ? undefined : 'button'}
                  onPress={() => {
                    tapFeedback();
                    router.push(personHref(b.userId, b.name));
                  }}
                  style={({ pressed }) => [styles.balanceRow, pressed && { opacity: 0.7 }]}>
                  <Avatar name={b.name} id={b.userId} size={36} />
                  <View style={styles.flex1}>
                    <Txt weight="600" numberOfLines={1}>
                      {who(b.userId, b.name, myId)}
                    </Txt>
                    <Txt variant="small">{b.netCents > 0 ? 'gets back' : b.netCents < 0 ? 'owes' : 'settled up'}</Txt>
                  </View>
                  <View
                    style={[
                      styles.amountPill,
                      { backgroundColor: b.netCents > 0 ? c.positiveSoft : b.netCents < 0 ? c.negativeSoft : c.cardAlt },
                    ]}>
                    <Money cents={b.netCents} currency={currency} signed colored />
                  </View>
                  {b.userId === myId ? <View style={styles.chevronSpace} /> : <Icon name="chevron-forward" size={16} color={c.textMuted} />}
                </Pressable>
              ))
            )}
            {(balances.data.suggestedTransfers ?? []).length > 0 ? (
              <>
                <Txt variant="label" style={{ marginTop: spacing.sm }}>
                  Suggested transfers
                </Txt>
                {(balances.data.suggestedTransfers ?? []).map((t, i) => {
                  const iOwe = t.fromUserId === myId;
                  const owedToMe = t.toUserId === myId;
                  const busy =
                    settle.isPending &&
                    settle.variables?.fromUserId === t.fromUserId &&
                    settle.variables?.toUserId === t.toUserId;
                  const other = iOwe ? { id: t.toUserId, name: t.toName } : owedToMe ? { id: t.fromUserId, name: t.fromName } : null;
                  return (
                    <View key={`${t.fromUserId}-${t.toUserId}-${i}`} style={[styles.transfer, { backgroundColor: c.cardAlt }]}>
                      <Pressable
                        disabled={!other}
                        accessibilityRole={other ? 'button' : undefined}
                        accessibilityHint={other ? `Opens your shared account with ${other.name || 'them'}` : undefined}
                        onPress={() => {
                          if (!other) return;
                          tapFeedback();
                          router.push(personHref(other.id, other.name));
                        }}
                        style={({ pressed }) => [styles.transferHead, pressed && { opacity: 0.7 }]}>
                        <Avatar name={t.fromName} id={t.fromUserId} size={28} />
                        <Icon name="arrow-forward" size={16} color={c.textMuted} />
                        <Avatar name={t.toName} id={t.toUserId} size={28} />
                        <Txt style={styles.flex1} numberOfLines={2}>
                          <Txt weight="700">{who(t.fromUserId, t.fromName, myId)}</Txt>
                          {iOwe ? ' owe ' : ' owes '}
                          <Txt weight="700">{who(t.toUserId, t.toName, myId)}</Txt>
                        </Txt>
                        <Money cents={t.amountCents} currency={currency} color={iOwe ? c.negative : t.toUserId === myId ? c.positive : undefined} />
                        {other ? <Icon name="chevron-forward" size={16} color={c.textMuted} /> : null}
                      </Pressable>
                      {iOwe ? (
                        <>
                          <PayButtons
                            handles={t.toPayment}
                            amountCents={t.amountCents}
                            currency={currency}
                            note={`Billsplit: ${g.name}`}
                            recipientName={t.toName || 'them'}
                            onMarkPaid={() => settle.mutate(t)}
                          />
                          <Button
                            title="I paid"
                            small
                            icon="checkmark-done"
                            loading={busy}
                            onPress={async () => {
                              const ok = await confirm(
                                'I paid',
                                `Record that you paid ${t.toName || 'them'} ${formatMoney(t.amountCents, currency)}?`,
                                'Record payment',
                              );
                              if (ok) settle.mutate(t);
                            }}
                          />
                        </>
                      ) : owedToMe ? (
                        <Button
                          title="Mark as received"
                          small
                          variant="secondary"
                          icon="checkmark-done"
                          loading={busy}
                          onPress={async () => {
                            const ok = await confirm(
                              'Mark as received',
                              `Record that ${t.fromName || 'they'} paid you ${formatMoney(t.amountCents, currency)}?`,
                              'Record payment',
                            );
                            if (ok) settle.mutate(t);
                          }}
                        />
                      ) : null}
                    </View>
                  );
                })}
              </>
            ) : (balances.data.balances ?? []).length > 0 ? (
              <Banner kind="success" message="Everyone is settled up." />
            ) : null}
          </>
        ) : balances.error ? (
          <ErrorView error={balances.error} onRetry={() => void balances.refetch()} />
        ) : (
          <SkeletonList rows={2} card={false} />
        )}
      </Card>

      {/* Receipts */}
      <SectionHeader title="Receipts & expenses" />
      {receipts.data ? (
        (receipts.data.receipts ?? []).length === 0 ? (
          <Card>
            <EmptyView
              icon="receipt"
              title="Nothing here yet"
              message="Scan a receipt, enter one manually or add a simple expense — everyone claims what they had."
              action={
                <Button
                  title="Scan a receipt"
                  icon="scan"
                  small
                  onPress={() => router.push(`/group/${groupId}/add-receipt?mode=scan`)}
                />
              }
            />
          </Card>
        ) : (
          <Card>
            {(receipts.data.receipts ?? []).map((r: Receipt, i) => {
              const expense = r.kind === 'RECEIPT_KIND_EXPENSE';
              return (
                <FadeIn key={r.id} index={i}>
                <ListRow
                  divider={i > 0}
                  onPress={() => router.push(`/receipt/${r.id}`)}
                  left={
                    <IconTile
                      icon={expense ? 'cash' : 'receipt'}
                      color={expense ? tone.ochre : tone.forest}
                      solid
                      size={42}
                    />
                  }
                  title={r.store || (expense ? 'Expense' : 'Receipt')}
                  subtitle={`${expense ? 'Expense · ' : ''}Paid by ${who(r.payerId, r.payerName, myId)} · ${formatDate(
                    r.purchasedAt || r.createdAt,
                  )}`}
                  right={
                    <View style={styles.receiptRight}>
                      <Money cents={r.totalCents} currency={r.currency || g.currency} />
                      {expense ? null : <StatusBadge status={r.status} />}
                    </View>
                  }
                />
                </FadeIn>
              );
            })}
          </Card>
        )
      ) : receipts.error ? (
        <ErrorView error={receipts.error} onRetry={() => void receipts.refetch()} />
      ) : (
        <SkeletonList rows={3} />
      )}

      {/* Members */}
      <SectionHeader title={`Members (${(g.members ?? []).length})`} action="+ Add friend" onAction={() => setAddOpen(true)} />
      <Card>
        {(g.members ?? []).map((m, i) => (
          <ListRow
            key={m.userId}
            divider={i > 0}
            left={<Avatar name={m.name} id={m.userId} />}
            title={
              <Row gap={6}>
                <Txt weight="600" numberOfLines={1} style={styles.shrink}>
                  {m.userId === myId ? `${m.name || m.email} (you)` : m.name || m.email}
                </Txt>
                {m.role === 'owner' ? <Badge label="Owner" tone="brand" icon="star" /> : null}
              </Row>
            }
            subtitle={m.email}
            onPress={m.userId === myId ? undefined : () => router.push(personHref(m.userId, m.name || m.email))}
            right={
              isOwner && m.userId !== myId ? (
                <IconButton
                  icon="person-remove-outline"
                  color={c.danger}
                  label="Remove member"
                  onPress={async () => {
                    const ok = await confirm('Remove member', `Remove ${m.name || m.email} from ${g.name}?`, 'Remove', true);
                    if (ok) removeMember.mutate(m.userId);
                  }}
                />
              ) : m.userId !== myId ? (
                <Icon name="chevron-forward" size={16} color={c.textMuted} />
              ) : undefined
            }
          />
        ))}
      </Card>
      <SelectModal
        visible={addOpen}
        title="Add a friend"
        options={addable}
        onSelect={(userId) => addMember.mutate(userId)}
        onClose={() => setAddOpen(false)}
        searchable
      />

      {/* Invites */}
      <SectionHeader title="Invite links" />
      <Card>
        <Txt variant="muted">Anyone with an active link can join this group.</Txt>
        {invites.data ? (
          (invites.data.invites ?? []).map((inv) => {
            const url = inv.url || inv.appUrl;
            return (
              <View key={inv.token} style={[styles.linkBox, { backgroundColor: c.cardAlt }]}>
                <Txt variant="muted" numberOfLines={2} color={c.text}>
                  {url}
                </Txt>
                <Txt variant="small">
                  {inv.expiresAt ? `Expires ${formatDate(inv.expiresAt)}` : 'No expiry'} · {inv.uses}
                  {inv.maxUses > 0 ? ` / ${inv.maxUses}` : ''} {inv.uses === 1 ? 'use' : 'uses'}
                </Txt>
                <Row gap={spacing.sm}>
                  <Button
                    title="Share"
                    icon="share-outline"
                    small
                    style={styles.flex1}
                    onPress={() => void shareLink(url, `Join "${g.name}" on Billsplit`)}
                  />
                  <Button
                    title="Copy"
                    icon="copy-outline"
                    small
                    variant="secondary"
                    style={styles.flex1}
                    onPress={() => void copyText(url)}
                  />
                  <Button
                    title="Revoke"
                    small
                    variant="danger"
                    loading={revokeInvite.isPending && revokeInvite.variables === inv.token}
                    onPress={async () => {
                      const ok = await confirm('Revoke link', 'People will no longer be able to join with this link.', 'Revoke', true);
                      if (ok) revokeInvite.mutate(inv.token);
                    }}
                  />
                </Row>
              </View>
            );
          })
        ) : invites.error ? (
          <ErrorView error={invites.error} onRetry={() => void invites.refetch()} />
        ) : (
          <Skeleton height={90} r={radius.md} />
        )}
        <Button
          title={(invites.data?.invites ?? []).length > 0 ? 'Create another link' : 'Create invite link'}
          icon="link-outline"
          variant="secondary"
          loading={createInvite.isPending}
          onPress={() => createInvite.mutate()}
        />
        <Txt variant="small">New links are valid for 72 hours with unlimited uses.</Txt>
      </Card>

      {isOwner ? null : (
        <Button
          title="Leave group"
          icon="exit-outline"
          variant="danger"
          loading={removeMember.isPending && removeMember.variables === myId}
          onPress={async () => {
            const ok = await confirm('Leave group', `Leave ${g.name}? You will lose access to its receipts.`, 'Leave', true);
            if (ok) removeMember.mutate(myId);
          }}
        />
      )}
      <Celebration
        visible={!!celebrate}
        icon="handshake"
        title="Settled up!"
        message={celebrate ?? undefined}
        onClose={() => setCelebrate(null)}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  balanceRow: { paddingVertical: 6, flexDirection: 'row', alignItems: 'center', gap: spacing.md, minHeight: 48 },
  chevronSpace: { width: 16 },
  transferHead: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm, minHeight: 44 },
  amountPill: { paddingHorizontal: 10, paddingVertical: 4, borderRadius: radius.pill },
  shrink: { flexShrink: 1 },
  action: {
    flex: 1,
    alignItems: 'center',
    gap: 8,
    paddingVertical: spacing.md,
    borderRadius: radius.lg,
    borderWidth: StyleSheet.hairlineWidth,
  },
  coverEmoji: {
    width: 60,
    height: 60,
    borderRadius: 20,
    backgroundColor: 'rgba(255,255,255,0.22)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  heroRow: { marginTop: spacing.sm, alignItems: 'center' },
  heroAvatars: { paddingLeft: spacing.sm },
  glassSkel: { backgroundColor: 'rgba(255,255,255,0.2)' },
  transfer: { padding: spacing.md, borderRadius: radius.md, gap: spacing.sm },
  badge: { paddingHorizontal: 8, paddingVertical: 2, borderRadius: radius.pill },
  receiptRight: { alignItems: 'flex-end', gap: 4 },
  linkBox: { padding: spacing.md, borderRadius: radius.md, gap: spacing.sm },
});
