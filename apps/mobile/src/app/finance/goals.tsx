import { Icon } from '@/components/Icon';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useGoals, usePortfolio } from '@/api/queries';
import { FinanceApi } from '@/api/services';
import type { SavingsGoal } from '@/api/types';
import { useAuth } from '@/auth/AuthContext';
import { ProgressBar, Ring } from '@/components/charts';
import {
  Banner,
  Button,
  Card,
  EmptyView,
  ErrorView,
  FadeIn,
  Field,
  Row,
  Screen,
  SectionHeader,
  SelectField,
  SkeletonList,
  Txt,
} from '@/components/ui';
import { confirm } from '@/lib/dialogs';
import {
  amountPlaceholder,
  dateInputToRfc3339,
  formatDate,
  formatMoney,
  formatPercent,
  minorToInput,
  parseMinor,
  shortMonthYear,
  tsToDateInput,
} from '@/lib/format';
import { spacing, useTheme } from '@/theme';

function GoalForm({ goal, currency, onDone }: { goal: SavingsGoal | null; currency: string; onDone: () => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState(goal?.name ?? '');
  const [target, setTarget] = useState(goal ? minorToInput(goal.targetCents, currency) : '');
  const [saved, setSaved] = useState(goal ? minorToInput(goal.savedCents, currency) : '');
  const [deadline, setDeadline] = useState(tsToDateInput(goal?.deadline));
  const [linked, setLinked] = useState(goal?.investmentId ?? '');
  const [error, setError] = useState<string | null>(null);
  const portfolio = usePortfolio();
  const investments = portfolio.data?.items ?? [];
  const linkOptions = [
    { value: '', label: 'Manual amount', hint: 'You update "saved so far" yourself' },
    ...investments.map((i) => ({ value: i.id, label: i.name, hint: `${formatMoney(i.valueHomeCents, currency)} now` })),
  ];

  const save = useMutation({
    mutationFn: () => {
      if (!name.trim()) throw new Error('Name is required');
      const t = parseMinor(target, currency);
      if (t === null || t <= 0) throw new Error('Enter a positive target');
      const s = linked ? 0 : saved.trim() ? parseMinor(saved, currency) : 0;
      if (s === null || s < 0) throw new Error('Invalid saved amount');
      let dl: string | undefined;
      if (deadline.trim()) {
        const parsed = dateInputToRfc3339(deadline);
        if (!parsed) throw new Error('Deadline must be YYYY-MM-DD');
        dl = parsed;
      }
      return FinanceApi.setGoal({
        id: goal?.id || undefined,
        name: name.trim(),
        targetCents: t,
        savedCents: s,
        deadline: dl,
        investmentId: linked || undefined,
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.goals });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
      onDone();
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const del = useMutation({
    mutationFn: () => FinanceApi.deleteGoal({ id: goal?.id ?? '' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: qk.goals });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
      onDone();
    },
    onError: (e) => setError(errorMessage(e)),
  });

  return (
    <Card>
      <Txt variant="h3">{goal ? 'Edit goal' : 'New goal'}</Txt>
      {error ? <Banner kind="error" message={error} /> : null}
      <Field label="Name" value={name} onChangeText={setName} placeholder="Emergency fund" />
      {investments.length > 0 || linked ? (
        <SelectField label="Track progress with" value={linked} options={linkOptions} onChange={setLinked} />
      ) : null}
      <Row gap={spacing.sm}>
        <Field style={styles.flex1} label="Target" value={target} onChangeText={setTarget} keyboardType="decimal-pad" placeholder={amountPlaceholder(currency)} />
        {linked ? null : (
          <Field style={styles.flex1} label="Saved so far" value={saved} onChangeText={setSaved} keyboardType="decimal-pad" placeholder={amountPlaceholder(currency)} />
        )}
      </Row>
      {linked ? (
        <Txt variant="small">Progress follows the investment&apos;s value — deposit interest and top-ups count automatically.</Txt>
      ) : null}
      <Field label="Deadline (YYYY-MM-DD, optional)" value={deadline} onChangeText={setDeadline} placeholder="2027-06-30" autoCapitalize="none" />
      <Row gap={spacing.sm}>
        <Button title="Cancel" variant="ghost" style={styles.flex1} onPress={onDone} />
        <Button title="Save" icon="checkmark" style={styles.flex1} loading={save.isPending} onPress={() => { setError(null); save.mutate(); }} />
      </Row>
      {goal?.id ? (
        <Button
          title="Delete goal"
          icon="trash-outline"
          variant="danger"
          small
          loading={del.isPending}
          onPress={async () => {
            const ok = await confirm('Delete goal', `Delete "${goal.name}"?`, 'Delete', true);
            if (ok) del.mutate();
          }}
        />
      ) : null}
    </Card>
  );
}

export default function GoalsScreen() {
  const { c } = useTheme();
  const { user } = useAuth();
  const goals = useGoals();
  const [editing, setEditing] = useState<SavingsGoal | 'new' | null>(null);
  const cur = user?.currency || 'USD';

  return (
    <Screen refreshing={goals.isRefetching} onRefresh={() => void goals.refetch()}>
      {editing ? (
        <GoalForm
          key={editing === 'new' ? 'new' : editing.id}
          goal={editing === 'new' ? null : editing}
          currency={cur}
          onDone={() => setEditing(null)}
        />
      ) : (
        <Button title="New goal" icon="add" onPress={() => setEditing('new')} />
      )}

      <SectionHeader title="Your goals" />
      {goals.data ? (
        (goals.data.goals ?? []).length === 0 ? (
          <Card>
            <EmptyView icon="target" title="No savings goals" message="Set a target and we'll tell you how much to save each month." />
          </Card>
        ) : (
          (goals.data.goals ?? []).map((g, i) => {
            const progress = g.targetCents > 0 ? g.savedCents / g.targetCents : 0;
            return (
              <FadeIn key={g.id} index={i}>
              <Card onPress={() => setEditing(g)}>
                <Row gap={spacing.md}>
                  <Ring value={progress} size={64} stroke={7} color={progress >= 1 ? c.positive : c.savings} label={formatPercent(Math.min(progress, 1))} />
                  <View style={styles.flex1}>
                    <Txt variant="h3">{g.name}</Txt>
                    <Txt variant="muted">
                      {formatMoney(g.savedCents, cur)} of {formatMoney(g.targetCents, cur)}
                    </Txt>
                    {g.deadline ? <Txt variant="small">by {formatDate(g.deadline)}</Txt> : null}
                    {g.investmentId ? (
                      <Row gap={4}>
                        <Icon name="link" size={13} color={c.primary} />
                        <Txt variant="small" color={c.primary} weight="600" numberOfLines={1} style={styles.flex1}>
                          {g.investmentName}
                        </Txt>
                      </Row>
                    ) : null}
                  </View>
                </Row>
                <ProgressBar value={Math.min(progress, 1)} color={c.savings} height={10} />
                {g.investmentId && progress < 1 ? (
                  <Txt variant="small">
                    {g.projectedAt ? (
                      <>
                        At this rate you&apos;ll reach it by{' '}
                        <Txt variant="small" weight="800" color={c.text}>
                          {shortMonthYear(g.projectedAt.slice(0, 10))}
                        </Txt>
                        .
                      </>
                    ) : (
                      'Not reached with the current balance and top-ups — add more or extend the deposit.'
                    )}
                  </Txt>
                ) : null}
                {g.monthlyNeededCents > 0 ? (
                  <Txt variant="small">
                    Save <Txt variant="small" weight="800" color={c.text}>{formatMoney(g.monthlyNeededCents, cur)}</Txt> per month to reach it on time.
                  </Txt>
                ) : progress >= 1 ? (
                  <Txt variant="small" color={c.positive}>Goal reached</Txt>
                ) : null}
              </Card>
              </FadeIn>
            );
          })
        )
      ) : goals.error ? (
        <ErrorView error={goals.error} onRetry={() => void goals.refetch()} />
      ) : (
        <SkeletonList rows={2} />
      )}
      <Txt variant="small">Amounts are shown in your home currency ({cur}).</Txt>
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
});
