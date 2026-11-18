import { Icon } from '@/components/Icon';
import { useRouter } from 'expo-router';
import React from 'react';
import { StyleSheet, View } from 'react-native';

import { useGroups, useMyBalances } from '@/api/queries';
import { useAuth } from '@/auth/AuthContext';
import {
  Avatar,
  AvatarStack,
  Badge,
  Button,
  Card,
  Columns,
  EmptyView,
  ErrorView,
  FadeIn,
  GlassPill,
  HeroCard,
  IconTile,
  ListRow,
  Money,
  Row,
  Screen,
  SectionHeader,
  Skeleton,
  SkeletonList,
  Txt,
} from '@/components/ui';
import { groupCover } from '@/lib/colors';
import { personHref } from '@/lib/person';
import { onHero, radius, spacing, useTheme } from '@/theme';

const WHITE = '#FFFFFF';
const WHITE_SOFT = 'rgba(255,255,255,0.82)';

function greeting(): string {
  const h = new Date().getHours();
  if (h < 5) return 'Good night';
  if (h < 12) return 'Good morning';
  if (h < 18) return 'Good afternoon';
  return 'Good evening';
}

export default function GroupsScreen() {
  const router = useRouter();
  const { c } = useTheme();
  const { user } = useAuth();
  const groups = useGroups();
  const balances = useMyBalances();

  const refreshing = groups.isRefetching || balances.isRefetching;
  const onRefresh = () => {
    void groups.refetch();
    void balances.refetch();
  };
  const b = balances.data;
  const net = b ? b.owedToMeCents - b.iOweCents : 0;
  const firstName = (user?.name || '').split(' ')[0];
  const people = (b?.friends ?? [])
    .filter((f) => (f.netCents ?? 0) !== 0)
    .sort((x, y) => Math.abs(y.netCents) - Math.abs(x.netCents));
  const PEOPLE_MAX = 6;

  return (
    <Screen refreshing={refreshing} onRefresh={onRefresh} dashboard>
      <Columns
        left={
          <>
            <HeroCard
              onPress={
                people.length === 1
                  ? () => router.push(personHref(people[0].userId, people[0].name))
                  : people.length > 1
                    ? () => router.push('/friends')
                    : undefined
              }>
              <Row>
                <Txt variant="small" color={WHITE_SOFT} weight="600" style={styles.flex1}>
                  {greeting()}
                  {firstName ? `, ${firstName}` : ''}
                </Txt>
                {people.length > 0 ? <Icon name="chevron-forward" size={18} color={WHITE_SOFT} /> : null}
              </Row>
              <Txt variant="label" color={WHITE_SOFT}>
                Total balance
              </Txt>
              {b ? (
                <>
                  <Money cents={net} currency={b.currency} signed variant="hero" color={WHITE} animate />
                  <Txt variant="small" color={WHITE_SOFT}>
                    {net > 0 ? 'Overall, friends owe you' : net < 0 ? 'Overall, you owe friends' : 'You are all settled up'}
                  </Txt>
                  <Row gap={spacing.sm} style={styles.pills}>
                    <GlassPill>
                      <Row gap={6}>
                        <View style={[styles.arrow, styles.arrowChip]}>
                          <Icon name="arrow-down" size={13} color={WHITE} />
                        </View>
                        <Txt variant="small" color={WHITE_SOFT} weight="600">
                          Owed to you
                        </Txt>
                      </Row>
                      <Money cents={b.owedToMeCents} currency={b.currency} variant="h3" color={WHITE} />
                    </GlassPill>
                    <GlassPill>
                      <Row gap={6}>
                        <View style={[styles.arrow, styles.arrowChip]}>
                          <Icon name="arrow-up" size={13} color={WHITE} />
                        </View>
                        <Txt variant="small" color={WHITE_SOFT} weight="600">
                          You owe
                        </Txt>
                      </Row>
                      <Money cents={b.iOweCents} currency={b.currency} variant="h3" color={WHITE} />
                    </GlassPill>
                  </Row>
                </>
              ) : balances.error ? (
                <Txt color={WHITE}>Balances unavailable — pull to refresh</Txt>
              ) : (
                <View style={styles.heroSkel}>
                  <Skeleton width={180} height={38} style={styles.glassSkel} />
                  <Row gap={spacing.sm}>
                    <Skeleton width="48%" height={58} style={styles.glassSkel} />
                    <Skeleton width="48%" height={58} style={styles.glassSkel} />
                  </Row>
                </View>
              )}
            </HeroCard>

            {people.length > 0 ? (
              <>
                <SectionHeader
                  title="People"
                  action={people.length > PEOPLE_MAX ? `All ${people.length}` : undefined}
                  icon="people"
                  onAction={() => router.push('/friends')}
                />
                <Card>
                  {people.slice(0, PEOPLE_MAX).map((p, i) => (
                    <FadeIn key={p.userId} index={i}>
                      <ListRow
                        divider={i > 0}
                        onPress={() => router.push(personHref(p.userId, p.name))}
                        left={<Avatar name={p.name} id={p.userId} />}
                        title={p.name || 'Group member'}
                        subtitle={
                          <Row gap={6}>
                            <Badge
                              label={p.netCents > 0 ? 'owes you' : 'you owe'}
                              tone={p.netCents > 0 ? 'ok' : 'bad'}
                              icon={p.netCents > 0 ? 'arrow-down' : 'arrow-up'}
                            />
                          </Row>
                        }
                        right={
                          <Row gap={4}>
                            <View style={[styles.amountPill, { backgroundColor: p.netCents > 0 ? c.positiveSoft : c.negativeSoft }]}>
                              <Money cents={p.netCents} currency={b?.currency || 'USD'} signed colored />
                            </View>
                            <Icon name="chevron-forward" size={16} color={c.textMuted} />
                          </Row>
                        }
                      />
                    </FadeIn>
                  ))}
                </Card>
              </>
            ) : !b && !balances.error ? (
              <>
                <SectionHeader title="People" />
                <SkeletonList rows={2} />
              </>
            ) : null}
          </>
        }
        right={
          <>
            <SectionHeader title="Your groups" action="New group" icon="add" onAction={() => router.push('/group/new')} />

            {groups.data ? (
              groups.data.groups.length === 0 ? (
                <Card>
                  <EmptyView
                    icon="people"
                    title="No groups yet"
                    message="Create a group for a trip, your flat or tonight's dinner — then scan the receipt and let everyone claim their items."
                    action={<Button title="Create your first group" icon="add" onPress={() => router.push('/group/new')} />}
                  />
                </Card>
              ) : (
                groups.data.groups.map((g, i) => {
                  const cover = groupCover(g.id, g.name);
                  const members = g.members ?? [];
                  return (
                    <FadeIn key={g.id} index={i}>
                      <Card onPress={() => router.push(`/group/${g.id}`)}>
                        <Row gap={spacing.md}>
                          <IconTile icon={cover.icon} color={cover.color} solid size={50} />
                          <View style={styles.flex1}>
                            <Txt variant="h3" numberOfLines={1}>
                              {g.name}
                            </Txt>
                            <Row gap={6}>
                              <Txt variant="small">
                                {members.length} {members.length === 1 ? 'member' : 'members'}
                              </Txt>
                              <View style={[styles.dot, { backgroundColor: c.textMuted }]} />
                              <Txt variant="small">{g.currency}</Txt>
                            </Row>
                          </View>
                          <AvatarStack people={members.map((m) => ({ id: m.userId, name: m.name }))} size={28} max={3} />
                          <Icon name="chevron-forward" size={18} color={c.textMuted} />
                        </Row>
                      </Card>
                    </FadeIn>
                  );
                })
              )
            ) : groups.error ? (
              <ErrorView error={groups.error} onRetry={() => void groups.refetch()} />
            ) : (
              <SkeletonList rows={3} />
            )}
          </>
        }
      />
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  pills: { marginTop: spacing.sm },
  arrow: { width: 20, height: 20, borderRadius: 10, alignItems: 'center', justifyContent: 'center' },
  arrowChip: { backgroundColor: onHero.chip },
  dot: { width: 3, height: 3, borderRadius: 2 },
  heroSkel: { gap: spacing.md },
  glassSkel: { backgroundColor: 'rgba(255,255,255,0.18)' },
  amountPill: { paddingHorizontal: 10, paddingVertical: 4, borderRadius: radius.pill },
});
