import { Icon } from '@/components/Icon';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useFriendLedger } from '@/api/queries';
import { BalanceApi } from '@/api/services';
import type { FriendLedger, SettleWithFriendResponse } from '@/api/types';
import { useMyId } from '@/auth/AuthContext';
import { Celebration } from '@/components/Celebration';
import { LedgerGroupCard, NettingCard, SettleSheet } from '@/components/ledger';
import { PayButtons } from '@/components/PayButtons';
import {
  Avatar,
  Banner,
  Button,
  Card,
  EmptyView,
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
  Txt,
} from '@/components/ui';
import { formatDate, formatMoney } from '@/lib/format';
import { firstName, netPhrase, reminderText } from '@/lib/person';
import { shareText } from '@/lib/share';
import { radius, spacing, tone } from '@/theme';

const WHITE = '#FFFFFF';
const WHITE_SOFT = 'rgba(255,255,255,0.86)';

/** Hero color by direction: brick — you owe; the brand gradient — they owe you or settled. */
function heroColor(net: number): string | undefined {
  return net < 0 ? tone.brick : undefined;
}

function lastActivity(l: FriendLedger): string {
  let best = '';
  for (const g of l.groups) for (const e of g.entries) if (e.occurredAt && e.occurredAt > best) best = e.occurredAt;
  return best ? formatDate(best) : '—';
}

function PersonSkeleton() {
  return (
    <>
      <Skeleton height={236} r={radius.xl} />
      <Skeleton height={112} r={radius.lg} />
      <SkeletonList rows={2} />
      <SkeletonList rows={3} />
    </>
  );
}

export default function PersonScreen() {
  const params = useLocalSearchParams<{ id: string; name?: string }>();
  const userId = String(params.id ?? '');
  const router = useRouter();
  const qc = useQueryClient();
  const myId = useMyId();
  const ledger = useFriendLedger(userId === myId ? '' : userId);

  const [sheet, setSheet] = useState(false);
  const [sheetError, setSheetError] = useState<string | null>(null);
  const [done, setDone] = useState<{ title: string; message: string } | null>(null);

  const l = ledger.data;
  const fullName = l?.name || params.name || 'Account';
  const first = firstName(l?.name || params.name, 'them');

  const settle = useMutation({
    mutationFn: () => BalanceApi.settleWithFriend({ userId }),
    onSuccess: (res: SettleWithFriendResponse) => {
      const n = (res.settlements ?? []).length;
      const paid = res.netPaidCents ?? 0;
      const cur = res.currency || l?.currency || 'USD';
      const groups = `${n} ${n === 1 ? 'group' : 'groups'}`;
      setSheet(false);
      setSheetError(null);
      setDone({
        title: paid > 0 ? 'Money in!' : 'Settled up!',
        message:
          paid > 0
            ? `${first} paid you ${formatMoney(paid, cur)} — ${groups} closed. You're square!`
            : paid < 0
              ? `You paid ${first} ${formatMoney(-paid, cur)} — ${groups} closed. You're square!`
              : `Balances with ${first} closed out in ${groups}. You're square!`,
      });
      void qc.invalidateQueries({ queryKey: qk.friendLedgerAll });
      void qc.invalidateQueries({ queryKey: qk.myBalances });
      void qc.invalidateQueries({ queryKey: ['groupBalances'] });
      void qc.invalidateQueries({ queryKey: qk.notifications });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
    },
    onError: (e) => {
      // Also surfaces errors from the pay-button "Mark as paid?" path, where the sheet is closed.
      setSheetError(errorMessage(e));
      setSheet(true);
    },
  });

  const openSheet = () => {
    setSheetError(null);
    setSheet(true);
  };

  const header = <Stack.Screen options={{ title: fullName }} />;

  if (userId && userId === myId) {
    return (
      <Screen>
        {header}
        <EmptyView icon="person" title="That's you!" message="Pick a friend to see your shared account with them." />
      </Screen>
    );
  }

  if (!l) {
    return (
      <Screen>
        {header}
        {ledger.error ? <ErrorView error={ledger.error} onRetry={() => void ledger.refetch()} /> : <PersonSkeleton />}
      </Screen>
    );
  }

  const net = l.netCents;
  const openGroups = l.groups.filter((g) => g.netCents !== 0);
  const groupNames = openGroups.map((g) => g.groupName).filter(Boolean);

  return (
    <Screen refreshing={ledger.isRefetching} onRefresh={() => void ledger.refetch()}>
      {header}

      <FadeIn>
        <HeroCard color={heroColor(net)}>
          <Row gap={spacing.md}>
            <Avatar name={fullName} id={userId} size={60} ring="rgba(255,255,255,0.9)" />
            <View style={styles.flex1}>
              <Txt variant="h2" color={WHITE} numberOfLines={1}>
                {fullName}
              </Txt>
              <Txt variant="small" color={WHITE_SOFT} weight="600">
                Shared account · all groups
              </Txt>
            </View>
          </Row>
          <View style={styles.amountBlock}>
            <Row gap={6}>
              <View style={styles.dirIcon}>
                <Icon
                  name={net > 0 ? 'arrow-down' : net < 0 ? 'arrow-up' : 'checkmark'}
                  size={13}
                  color={WHITE}
                />
              </View>
              <Txt variant="label" color={WHITE_SOFT}>
                {net === 0 ? 'Your balance' : netPhrase(net, first)}
              </Txt>
            </Row>
            {net === 0 ? (
              <Txt variant="hero" color={WHITE}>
                All settled
              </Txt>
            ) : (
              <Money cents={Math.abs(net)} currency={l.currency} variant="hero" color={WHITE} animate />
            )}
            <Txt variant="small" color={WHITE_SOFT}>
              {net === 0
                ? openGroups.length > 0
                  ? 'Your groups cancel each other out.'
                  : `Nothing owed either way with ${first}.`
                : `Net across ${l.groups.length} ${l.groups.length === 1 ? 'group' : 'groups'}, in ${l.currency}`}
            </Txt>
          </View>
          <Row gap={spacing.sm} style={styles.pills}>
            <GlassPill>
              <Txt variant="small" color={WHITE_SOFT} weight="600">
                Open groups
              </Txt>
              <Txt variant="h3" color={WHITE}>
                {openGroups.length} / {l.groups.length}
              </Txt>
            </GlassPill>
            <GlassPill>
              <Txt variant="small" color={WHITE_SOFT} weight="600">
                Last activity
              </Txt>
              <Txt variant="h3" color={WHITE} numberOfLines={1}>
                {lastActivity(l)}
              </Txt>
            </GlassPill>
          </Row>
        </HeroCard>
      </FadeIn>

      {/* Actions */}
      <FadeIn index={1}>
        {net < 0 ? (
          <Card>
            <Txt variant="h3">Pay {first}</Txt>
            <Txt variant="small">
              One payment of {formatMoney(-net, l.currency)} covers every group — then record it to close them all.
            </Txt>
            <PayButtons
              handles={l.payment}
              amountCents={-net}
              currency={l.currency}
              note="Billsplit"
              recipientName={first}
              onMarkPaid={() => settle.mutate()}
            />
            <Button title="Settle up all" icon="checkmark-done-circle" onPress={openSheet} />
          </Card>
        ) : net > 0 ? (
          <Card>
            <Txt variant="h3">{first} owes you</Txt>
            <Txt variant="small">Got the money? Record it once — it settles every group you share.</Txt>
            <Row gap={spacing.sm}>
              <Button title="Mark all as received" icon="checkmark-done" onPress={openSheet} style={styles.flex1} />
              <Button
                title="Remind"
                icon="paper-plane-outline"
                variant="secondary"
                onPress={() => void shareText(reminderText(fullName, net, l.currency, groupNames))}
              />
            </Row>
          </Card>
        ) : openGroups.length > 0 ? (
          <Card>
            <Banner
              kind="info"
              title="Even overall"
              message={`You owe in some groups and ${first} owes in others — it nets to zero. Close them out to clean up.`}
            />
            <Button
              title={`Close out ${openGroups.length} ${openGroups.length === 1 ? 'group' : 'groups'}`}
              icon="checkmark-done"
              variant="secondary"
              onPress={openSheet}
            />
          </Card>
        ) : l.groups.length > 0 ? (
          <Banner kind="success" message={`You and ${first} are all square. Nice!`} />
        ) : null}
      </FadeIn>

      {l.groups.length === 0 ? (
        <Card>
          <EmptyView
            icon="handshake"
            title="No shared history yet"
            message={`Add ${first} to a group and split a receipt — everything you share will add up here in one account.`}
            action={<Button title="Create a group" icon="add" small onPress={() => router.push('/group/new')} />}
          />
        </Card>
      ) : (
        <>
          <FadeIn index={2}>
            <NettingCard ledger={l} />
          </FadeIn>
          <SectionHeader title={`By group · ${l.groups.length}`} />
          {l.groups.map((g, i) => (
            <FadeIn key={g.groupId} index={i + 3}>
              <LedgerGroupCard
                group={g}
                homeCurrency={l.currency}
                friend={first}
                initiallyOpen={i === 0}
                onOpenReceipt={(rid) => router.push(`/receipt/${rid}`)}
                onOpenGroup={(gid) => router.push(`/group/${gid}`)}
              />
            </FadeIn>
          ))}
          <Txt variant="small" style={styles.footnote}>
            Amounts are from your side: + means {first} owes you more, − means you owe more.
          </Txt>
        </>
      )}

      <SettleSheet
        visible={sheet}
        ledger={l}
        friend={first}
        loading={settle.isPending}
        error={sheetError}
        onConfirm={() => settle.mutate()}
        onClose={() => {
          if (!settle.isPending) setSheet(false);
        }}
      />
      <Celebration
        visible={!!done}
        icon="handshake"
        title={done?.title ?? ''}
        message={done?.message}
        onClose={() => setDone(null)}
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  amountBlock: { marginTop: spacing.sm, gap: 2 },
  dirIcon: {
    width: 20,
    height: 20,
    borderRadius: 10,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: 'rgba(255,255,255,0.22)',
  },
  pills: { marginTop: spacing.sm },
  footnote: { textAlign: 'center', marginTop: spacing.xs },
});
