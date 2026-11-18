import { Icon } from '@/components/Icon';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useRef, useState } from 'react';
import { Pressable, StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useGroup, useReceipt, useSplit } from '@/api/queries';
import { ReceiptApi } from '@/api/services';
import type { Group, Receipt, ReceiptItem } from '@/api/types';
import { useAuth, useMyId } from '@/auth/AuthContext';
import { ProgressBar } from '@/components/charts';
import { Celebration } from '@/components/Celebration';
import {
  Avatar,
  AvatarStack,
  Badge,
  Banner,
  Button,
  Card,
  ErrorView,
  FadeIn,
  GlassPill,
  HeroCard,
  Money,
  Row,
  Screen,
  SectionHeader,
  Skeleton,
  SkeletonList,
  Stepper,
  Txt,
} from '@/components/ui';
import { tapFeedback } from '@/lib/haptics';
import { memberName } from '@/lib/members';
import { confirm, notify } from '@/lib/dialogs';
import { formatDate, formatMoney } from '@/lib/format';
import { useReceiptLive, type LiveEvent } from '@/lib/live';
import { statusLabel } from '@/lib/receipt';
import { useNow } from '@/lib/useNow';
import { radius, softShadow, spacing, useTheme } from '@/theme';

function myShares(item: ReceiptItem, myId: string): number {
  return item.claims?.find((cl) => cl.userId === myId)?.shares ?? 0;
}

function withClaim(r: Receipt, itemId: string, userId: string, shares: number): Receipt {
  return {
    ...r,
    items: r.items.map((it) => {
      if (it.id !== itemId) return it;
      const others = (it.claims ?? []).filter((cl) => cl.userId !== userId);
      return { ...it, claims: shares > 0 ? [...others, { userId, shares }] : others };
    }),
  };
}

/** "Live" strip: who else has the receipt open, and the latest change by someone else. */
function LiveBar({ viewers, group, note }: { viewers: string[]; group?: Group; note: { userId: string; text: string } | null }) {
  const { c } = useTheme();
  const names = viewers.map((v) => memberName(group, v));
  const here =
    names.length === 0
      ? 'Live · changes appear instantly'
      : names.length === 1
        ? `${names[0]} is here too`
        : `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]} are here`;
  return (
    <View style={[styles.liveBar, { backgroundColor: c.cardAlt }]}>
      <Row gap={8}>
        <View style={[styles.liveDot, { backgroundColor: c.positive }]} />
        <Txt variant="small" weight="600" color={c.text} style={styles.flex1} numberOfLines={1}>
          {here}
        </Txt>
        {viewers.length > 0 ? (
          <AvatarStack size={20} people={viewers.map((id) => ({ id, name: memberName(group, id) }))} />
        ) : null}
      </Row>
      {note ? (
        <FadeIn key={note.text}>
          <Row gap={8} style={styles.liveNote}>
            <Avatar id={note.userId} name={memberName(group, note.userId)} size={20} />
            <Txt variant="small" color={c.text} style={styles.flex1} numberOfLines={2}>
              {note.text}
            </Txt>
          </Row>
        </FadeIn>
      ) : null}
    </View>
  );
}

export default function ReceiptScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const receiptId = String(id ?? '');
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const now = useNow(5000);
  const myId = useMyId();
  const { user } = useAuth();

  const receipt = useReceipt(receiptId);
  const r = receipt.data;
  const group = useGroup(r?.groupId ?? '');
  const ready = r?.status === 'RECEIPT_STATUS_PARSED' || r?.status === 'RECEIPT_STATUS_FINALIZED';
  const split = useSplit(receiptId, !!ready);

  const [expanded, setExpanded] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [celebrate, setCelebrate] = useState(false);
  // Live: items someone else just touched glow briefly; a one-line note says what happened.
  const [flash, setFlash] = useState<Record<string, true>>({});
  const [note, setNote] = useState<{ userId: string; text: string } | null>(null);
  const noteTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const onLive = (ev: LiveEvent) => {
    if (ev.type === 'receipt.deleted' && ev.user_id !== myId) {
      notify('Receipt deleted', 'The payer deleted this receipt.');
      router.back();
      return;
    }
    if (ev.type !== 'receipt.updated' || !ev.user_id || ev.user_id === myId) return;
    const cached = qc.getQueryData<Receipt>(qk.receipt(receiptId));
    const gr = qc.getQueryData<Group>(qk.group(cached?.groupId ?? ''));
    const who = memberName(gr, ev.user_id);
    let text = '';
    if (ev.action === 'claim') {
      const ids = ev.item_ids ?? [];
      const names = ids.map((id) => cached?.items?.find((it) => it.id === id)?.name).filter(Boolean);
      const took = (ev.shares ?? []).some((x) => x > 0);
      text = `${who} ${took ? 'took' : 'dropped'} ${names.join(', ') || 'an item'}`;
      setFlash((f) => ({ ...f, ...Object.fromEntries(ids.map((id) => [id, true as const])) }));
      setTimeout(
        () =>
          setFlash((f) => {
            const next = { ...f };
            ids.forEach((id) => delete next[id]);
            return next;
          }),
        1600,
      );
    } else if (ev.action === 'items') text = `${who} edited the items`;
    else if (ev.action === 'finalized') text = `${who} finalized the split`;
    if (!text) return;
    setNote({ userId: ev.user_id, text });
    if (noteTimer.current) clearTimeout(noteTimer.current);
    noteTimer.current = setTimeout(() => setNote(null), 3500);
  };
  const live = useReceiptLive(receiptId, myId, onLive);

  const claim = useMutation({
    mutationFn: (v: { itemId: string; shares: number }) =>
      ReceiptApi.claim({ receiptId, claims: [{ itemId: v.itemId, shares: v.shares }] }),
    onMutate: async (v) => {
      await qc.cancelQueries({ queryKey: qk.receipt(receiptId) });
      const prev = qc.getQueryData<Receipt>(qk.receipt(receiptId));
      if (prev) qc.setQueryData(qk.receipt(receiptId), withClaim(prev, v.itemId, myId, v.shares));
      return { prev };
    },
    onError: (e, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(qk.receipt(receiptId), ctx.prev);
      setError(errorMessage(e));
    },
    onSuccess: (updated) => {
      qc.setQueryData(qk.receipt(receiptId), updated);
      setError(null);
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: qk.split(receiptId) });
    },
  });

  const finalize = useMutation({
    mutationFn: () => ReceiptApi.finalize({ receiptId }),
    onSuccess: (s) => {
      qc.setQueryData(qk.split(receiptId), s);
      void qc.invalidateQueries({ queryKey: qk.receipt(receiptId) });
      if (r) {
        void qc.invalidateQueries({ queryKey: qk.receipts(r.groupId) });
        void qc.invalidateQueries({ queryKey: qk.groupBalances(r.groupId) });
      }
      void qc.invalidateQueries({ queryKey: qk.myBalances });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
      void qc.invalidateQueries({ queryKey: qk.transactionsAll });
      setCelebrate(true);
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const remove = useMutation({
    mutationFn: () => ReceiptApi.remove({ receiptId }),
    onSuccess: () => {
      if (r) {
        void qc.invalidateQueries({ queryKey: qk.receipts(r.groupId) });
        void qc.invalidateQueries({ queryKey: qk.groupBalances(r.groupId) });
      }
      void qc.invalidateQueries({ queryKey: qk.myBalances });
      qc.removeQueries({ queryKey: qk.receipt(receiptId) });
      router.back();
    },
    onError: (e) => setError(errorMessage(e)),
  });

  if (!r) {
    return (
      <Screen>
        {receipt.error ? (
          <ErrorView error={receipt.error} onRetry={() => void receipt.refetch()} />
        ) : (
          <>
            <Skeleton height={170} r={radius.xl} />
            <SkeletonList rows={4} />
          </>
        )}
      </Screen>
    );
  }

  const g = group.data;
  const isPayer = r.payerId === myId;
  const finalized = r.status === 'RECEIPT_STATUS_FINALIZED';
  const pending = r.status === 'RECEIPT_STATUS_PENDING' || r.status === 'RECEIPT_STATUS_UNSPECIFIED';
  const failed = r.status === 'RECEIPT_STATUS_FAILED';
  const cur = r.currency || g?.currency || 'USD';
  const isExpense = r.kind === 'RECEIPT_KIND_EXPENSE';
  const payer = r.payerId === myId ? 'You' : r.payerName || 'Unknown';
  const title = r.store || (isExpense ? 'Expense' : 'Receipt');
  const unclaimed = new Set(split.data?.unclaimedItemIds ?? []);
  const status = statusLabel(r.status);
  const splitCur = split.data?.currency || cur;
  const me = (split.data?.shares ?? []).find((x) => x.userId === myId);
  const claimedShare = r.subtotalCents > 0 && split.data ? Math.max(0, 1 - split.data.unclaimedCents / r.subtotalCents) : 0;
  const myItems = (r.items ?? []).filter((it) => myShares(it, myId) > 0).length;
  const differs = !!split.data?.groupCurrency && !!split.data?.currency && split.data.groupCurrency !== split.data.currency;

  if (pending) {
    const waitedMs = now - Date.parse(r.createdAt ?? '');
    const slow = Number.isFinite(waitedMs) && waitedMs > 20_000;
    return (
      <Screen>
        <Stack.Screen options={{ title: 'Reading receipt…' }} />
        <HeroCard style={styles.pendingCard}>
          <Icon name="receipt" size={44} color={WHITE} weight="duotone" />
          <Txt variant="h2" color={WHITE}>
            Reading your receipt…
          </Txt>
          <Txt variant="muted" color={WHITE_SOFT} style={styles.center}>
            {slow
              ? 'This is taking longer than usual. We keep retrying automatically — or you can type the items in yourself.'
              : 'Usually a few seconds. We check every 2 seconds.'}
          </Txt>
        </HeroCard>
        <SkeletonList rows={4} />
        {slow && isPayer ? (
          <Button title="Enter items manually" icon="create-outline" onPress={() => router.push(`/receipt/${receiptId}/edit`)} />
        ) : null}
      </Screen>
    );
  }

  return (
    <Screen
      refreshing={receipt.isRefetching || split.isRefetching}
      onRefresh={() => {
        void receipt.refetch();
        if (ready) void split.refetch();
      }}>
      <Stack.Screen options={{ title }} />
      {error ? <Banner kind="error" message={error} /> : null}
      {r.parseError ? (
        <Banner kind={failed ? 'error' : 'warning'} title={failed ? 'Could not read receipt' : 'Check the items'} message={r.parseError} />
      ) : null}
      {failed ? (
        <Card>
          <Txt>Recognition failed. Enter the items manually to continue splitting.</Txt>
          {isPayer ? (
            <Button title="Edit items manually" icon="create-outline" onPress={() => router.push(`/receipt/${receiptId}/edit`)} />
          ) : (
            <Txt variant="muted">Only the payer can edit items.</Txt>
          )}
        </Card>
      ) : null}

      {/* Summary */}
      <HeroCard>
        <Row gap={spacing.md} style={styles.heroTop}>
          <View style={styles.heroIcon}>
            <Icon name={isExpense ? 'cash' : 'receipt'} size={28} color={WHITE} weight="duotone" />
          </View>
          <View style={styles.flex1}>
            <Txt variant="h2" color={WHITE} numberOfLines={2}>
              {title}
            </Txt>
            <Txt variant="small" color={WHITE_SOFT} weight="600">
              {formatDate(r.purchasedAt || r.createdAt)}
              {isExpense ? ' · Expense' : ` · ${status.label}`}
            </Txt>
          </View>
        </Row>
        <Txt variant="label" color={WHITE_SOFT}>
          Total
        </Txt>
        <Money cents={r.totalCents} currency={cur} variant="hero" color={WHITE} />
        <Row gap={spacing.sm}>
          <Avatar name={r.payerName} id={r.payerId} size={24} ring="rgba(255,255,255,0.85)" />
          <Txt variant="small" color={WHITE} weight="600">
            Paid by {payer}
          </Txt>
        </Row>
        {isExpense ? null : (
          <Row gap={spacing.sm} style={styles.heroPills}>
            <GlassPill>
              <Txt variant="small" color={WHITE_SOFT}>
                Subtotal
              </Txt>
              <Txt weight="700" color={WHITE} numberOfLines={1}>
                {formatMoney(r.subtotalCents, cur)}
              </Txt>
            </GlassPill>
            <GlassPill>
              <Txt variant="small" color={WHITE_SOFT}>
                Tax
              </Txt>
              <Txt weight="700" color={WHITE} numberOfLines={1}>
                {formatMoney(r.taxCents, cur)}
              </Txt>
            </GlassPill>
            <GlassPill>
              <Txt variant="small" color={WHITE_SOFT}>
                Tip
              </Txt>
              <Txt weight="700" color={WHITE} numberOfLines={1}>
                {formatMoney(r.tipCents, cur)}
              </Txt>
            </GlassPill>
          </Row>
        )}
      </HeroCard>

      {/* Your total */}
      {ready && split.data ? (
        <FadeIn>
          <Card style={[styles.youCard, { borderColor: me ? c.primary : c.border }]}>
            <Row gap={spacing.md}>
              <Avatar name={user?.name || me?.name} id={myId} size={44} />
              <View style={styles.flex1}>
                <Txt variant="label">Your total</Txt>
                <Money cents={me?.totalCents ?? 0} currency={splitCur} variant="big" color={me ? c.text : c.textMuted} animate fitChars={9} />
                {differs && me ? (
                  <Txt variant="small">≈ {formatMoney(me.totalGroupCents, split.data.groupCurrency)}</Txt>
                ) : null}
              </View>
              {isExpense ? null : (
                <Badge
                  label={myItems > 0 ? `${myItems} ${myItems === 1 ? 'item' : 'items'}` : 'Nothing yet'}
                  tone={myItems > 0 ? 'brand' : 'muted'}
                  icon={myItems > 0 ? 'checkmark-circle' : 'hand-left'}
                />
              )}
            </Row>
            {!isExpense && me ? (
              <Txt variant="small">
                Items {formatMoney(me.itemsCents, splitCur)} · Tax {formatMoney(me.taxCents, splitCur)} · Tip{' '}
                {formatMoney(me.tipCents, splitCur)}
              </Txt>
            ) : null}
            {!isExpense ? (
              <View style={styles.claimProgress}>
                <Row>
                  <Txt variant="small" style={styles.flex1}>
                    {split.data.unclaimedCents > 0
                      ? `${formatMoney(split.data.unclaimedCents, splitCur)} still unclaimed`
                      : 'Everything is claimed'}
                  </Txt>
                  <Txt variant="small" weight="700" color={c.text}>
                    {Math.round(claimedShare * 100)}%
                  </Txt>
                </Row>
                <ProgressBar value={claimedShare} color={split.data.unclaimedCents > 0 ? c.primary : c.positive} />
              </View>
            ) : null}
          </Card>
        </FadeIn>
      ) : null}

      {/* Items */}
      {isExpense ? null : (
        <>
          <SectionHeader title={finalized ? 'Items' : 'Tap what you had'} />
          {!finalized && live.connected ? <LiveBar viewers={live.viewers.filter((v) => v !== myId)} group={g} note={note} /> : null}
          {!finalized ? (
            <Txt variant="small" style={styles.hint}>
              Tap an item to claim it · long-press or ⋯ to split shares
            </Txt>
          ) : null}
          {(r.items ?? []).length === 0 ? (
            <Card>
              <Txt variant="muted">No items.</Txt>
            </Card>
          ) : (
            (r.items ?? []).map((it, idx) => {
              const mine = myShares(it, myId);
              const claimers = (it.claims ?? []).filter((cl) => cl.shares > 0);
              const isUnclaimed = unclaimed.has(it.id) || claimers.length === 0;
              const raw = (it.rawName ?? '').trim();
              const showRaw = !!raw && raw.toLowerCase() !== it.name.trim().toLowerCase();
              return (
                <FadeIn key={it.id} index={idx}>
                  <Pressable
                    disabled={finalized}
                    accessibilityRole="checkbox"
                    accessibilityState={{ checked: mine > 0, disabled: finalized }}
                    accessibilityLabel={`${it.name}, ${formatMoney(it.priceCents, cur)}`}
                    onPress={() => {
                      tapFeedback();
                      claim.mutate({ itemId: it.id, shares: mine > 0 ? 0 : 1 });
                    }}
                    onLongPress={() => setExpanded(expanded === it.id ? null : it.id)}
                    delayLongPress={350}
                    style={({ pressed }) => [
                      styles.item,
                      {
                        borderColor: flash[it.id] ? c.accent : mine > 0 ? c.primary : c.border,
                        backgroundColor: mine > 0 ? c.primarySoft : c.card,
                      },
                      flash[it.id] ? styles.itemFlash : null,
                      mine > 0 ? null : softShadow(c),
                      pressed && styles.itemPressed,
                    ]}>
                    <View
                      style={[
                        styles.check,
                        mine > 0
                          ? { backgroundColor: c.accent, borderColor: c.accent }
                          : { borderColor: isUnclaimed ? c.warning : c.border },
                      ]}>
                      {mine > 0 ? (
                        mine > 1 ? (
                          <Txt variant="small" weight="800" color={c.primaryText}>
                            {mine}
                          </Txt>
                        ) : (
                          <Icon name="checkmark" size={16} color={c.primaryText} />
                        )
                      ) : null}
                    </View>
                    <View style={styles.flex1}>
                      <Txt weight="700" numberOfLines={2}>
                        {it.name}
                        {it.quantity > 1 ? <Txt variant="muted"> ×{it.quantity}</Txt> : null}
                      </Txt>
                      {showRaw ? (
                        <Txt variant="small" numberOfLines={1} style={styles.raw}>
                          {raw}
                          {it.code ? ` · ${it.code}` : ''}
                        </Txt>
                      ) : null}
                      {claimers.length > 0 ? (
                        <Row gap={6} style={styles.claimers}>
                          <AvatarStack
                            size={22}
                            people={claimers.map((cl) => ({ id: cl.userId, name: memberName(g, cl.userId) }))}
                          />
                          <Txt variant="small" numberOfLines={1} style={styles.flex1}>
                            {claimers
                              .map((cl) => `${memberName(g, cl.userId, myId)}${cl.shares > 1 ? ` ×${cl.shares}` : ''}`)
                              .join(', ')}
                          </Txt>
                        </Row>
                      ) : (
                        <Txt variant="small" color={isUnclaimed ? c.warning : undefined} weight="600">
                          Unclaimed
                        </Txt>
                      )}
                    </View>
                    <Money cents={it.priceCents} currency={cur} />
                    {!finalized ? (
                      <Pressable
                        hitSlop={8}
                        accessibilityLabel="Split shares"
                        onPress={() => setExpanded(expanded === it.id ? null : it.id)}
                        style={styles.more}>
                        <Icon name="ellipsis-horizontal" size={18} color={c.textMuted} />
                      </Pressable>
                    ) : null}
                  </Pressable>
                  {expanded === it.id && !finalized ? (
                    <Row style={[styles.stepperRow, { backgroundColor: c.cardAlt }]}>
                      <Txt variant="muted" style={styles.flex1}>
                        My shares of this item
                      </Txt>
                      <Stepper value={mine} onChange={(v) => claim.mutate({ itemId: it.id, shares: v })} />
                    </Row>
                  ) : null}
                </FadeIn>
              );
            })
          )}
        </>
      )}

      {/* Split */}
      {ready ? (
        <>
          <SectionHeader title="Who pays what" />
          <Card>
            {split.data ? (
              <>
                {(split.data.shares ?? []).length === 0 ? (
                  <Txt variant="muted">Nobody has claimed anything yet.</Txt>
                ) : (
                  (split.data.shares ?? []).map((s) => {
                    const share = r.totalCents > 0 ? s.totalCents / r.totalCents : 0;
                    return (
                      <View key={s.userId} style={styles.shareRow}>
                        <Row gap={spacing.md}>
                          <Avatar name={s.name} id={s.userId} size={36} />
                          <View style={styles.flex1}>
                            <Txt weight="700" numberOfLines={1}>
                              {s.userId === myId ? 'You' : s.name || 'Unknown'}
                            </Txt>
                            {isExpense ? null : (
                              <Txt variant="small" numberOfLines={1}>
                                Items {formatMoney(s.itemsCents, splitCur)} · Tax {formatMoney(s.taxCents, splitCur)} · Tip{' '}
                                {formatMoney(s.tipCents, splitCur)}
                              </Txt>
                            )}
                          </View>
                          <View style={styles.right}>
                            <Money cents={s.totalCents} currency={splitCur} />
                            {differs ? (
                              <Txt variant="small">≈ {formatMoney(s.totalGroupCents, split.data.groupCurrency)}</Txt>
                            ) : null}
                          </View>
                        </Row>
                        <View style={styles.shareBar}>
                          <ProgressBar value={share} height={6} color={s.userId === myId ? c.accent : c.chartMuted} />
                        </View>
                      </View>
                    );
                  })
                )}
                {differs ? (
                  <Txt variant="small">
                    1 {split.data.currency} = {Number(split.data.fxRate || 0).toFixed(4)} {split.data.groupCurrency}
                  </Txt>
                ) : null}
              </>
            ) : split.error ? (
              <ErrorView error={split.error} onRetry={() => void split.refetch()} />
            ) : (
              <SkeletonList rows={2} card={false} />
            )}
          </Card>
        </>
      ) : null}

      {isPayer && !finalized ? (
        <>
          <Row gap={spacing.sm}>
            <Button
              title="Edit items"
              icon="create-outline"
              variant="secondary"
              style={styles.flex1}
              onPress={() => router.push(`/receipt/${receiptId}/edit`)}
            />
            <Button
              title="Finalize"
              icon="lock-closed"
              style={styles.flex1}
              disabled={!ready}
              loading={finalize.isPending}
              onPress={async () => {
                const warn =
                  split.data && split.data.unclaimedCents > 0
                    ? ` ${formatMoney(split.data.unclaimedCents, split.data.currency || cur)} is unclaimed.`
                    : '';
                const ok = await confirm(
                  'Finalize receipt',
                  `Debts will be added to group balances and items can no longer be changed.${warn}`,
                  'Finalize',
                );
                if (ok) finalize.mutate();
              }}
            />
          </Row>
          <Button
            title="Delete receipt"
            icon="trash-outline"
            variant="danger"
            loading={remove.isPending}
            onPress={async () => {
              const ok = await confirm('Delete receipt', 'This cannot be undone.', 'Delete', true);
              if (ok) remove.mutate();
            }}
          />
        </>
      ) : null}
      {finalized ? (
        <Banner
          kind="success"
          message={
            isExpense
              ? 'This expense is included in group balances.'
              : 'This receipt is finalized and included in group balances.'
          }
        />
      ) : null}
      <Celebration
        visible={celebrate}
        title="Receipt finalized!"
        message={
          me
            ? `Your part is ${formatMoney(me.totalCents, splitCur)}. Balances in the group are updated.`
            : 'Balances in the group are updated.'
        }
        onClose={() => setCelebrate(false)}
      />
    </Screen>
  );
}

const WHITE = '#FFFFFF';
const WHITE_SOFT = 'rgba(255,255,255,0.85)';

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  center: { textAlign: 'center' },
  right: { alignItems: 'flex-end' },
  pendingCard: { alignItems: 'center', paddingVertical: spacing.xxl, gap: spacing.md },
  heroTop: { marginBottom: spacing.xs },
  heroIcon: {
    width: 52,
    height: 52,
    borderRadius: 18,
    backgroundColor: 'rgba(255,255,255,0.22)',
    alignItems: 'center',
    justifyContent: 'center',
  },
  heroPills: { marginTop: spacing.sm },
  youCard: { borderWidth: 1.5 },
  claimProgress: { gap: 6, marginTop: spacing.xs },
  hint: { marginTop: -spacing.xs },
  item: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    padding: spacing.md,
    minHeight: 64,
    borderRadius: radius.lg,
    borderWidth: 1.5,
  },
  itemPressed: { transform: [{ scale: 0.985 }], opacity: 0.9 },
  check: {
    width: 28,
    height: 28,
    borderRadius: 14,
    borderWidth: 2,
    alignItems: 'center',
    justifyContent: 'center',
  },
  raw: { fontFamily: 'monospace', fontSize: 11.5, letterSpacing: 0.3 },
  claimers: { marginTop: 4 },
  itemFlash: { borderWidth: 2 },
  liveBar: { borderRadius: radius.md, paddingHorizontal: spacing.md, paddingVertical: 10, gap: 8, marginBottom: spacing.xs },
  liveDot: { width: 8, height: 8, borderRadius: 4 },
  liveNote: { alignItems: 'center' },
  more: { minWidth: 32, minHeight: 44, alignItems: 'center', justifyContent: 'center' },
  stepperRow: { padding: spacing.sm, paddingLeft: spacing.md, borderRadius: radius.md, marginTop: 6 },
  shareRow: { paddingVertical: 8, gap: 6 },
  shareBar: { marginLeft: 48 },
});
