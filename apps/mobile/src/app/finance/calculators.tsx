import { useLocalSearchParams } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import type { Capitalization } from '@/api/types';
import { useAuth } from '@/auth/AuthContext';
import { LineChart } from '@/components/charts';
import { Card, Field, Row, Screen, SectionHeader, Segmented, Txt } from '@/components/ui';
import { compactMoney, formatMoney, majorToMinor } from '@/lib/format';
import { CAPITALIZATIONS, compoundSchedule, depositSchedule, loanSchedule, LoanType, monthlyForTarget } from '@/lib/invest';
import { spacing, useTheme } from '@/theme';

type Tab = 'deposit' | 'invest' | 'loan';

const num = (s: string) => {
  const n = Number(s.replace(',', '.').replace(/\s/g, ''));
  return Number.isFinite(n) ? n : 0;
};

function Stat({ label, value, strong, color }: { label: string; value: string; strong?: boolean; color?: string }) {
  const { c } = useTheme();
  return (
    <Row style={styles.stat}>
      <Txt variant="muted" style={styles.flex1}>
        {label}
      </Txt>
      <Txt weight={strong ? '800' : '700'} color={color ?? c.text} style={styles.num}>
        {value}
      </Txt>
    </Row>
  );
}

/** Two numeric fields per row. */
function Pair({ children }: { children: React.ReactNode }) {
  return <Row gap={spacing.sm}>{children}</Row>;
}

function DepositCalc({ cur }: { cur: string }) {
  const { c } = useTheme();
  const [amount, setAmount] = useState('1000000');
  const [rate, setRate] = useState('16');
  const [months, setMonths] = useState('12');
  const [topUp, setTopUp] = useState('0');
  const [tax, setTax] = useState('0');
  const [cap, setCap] = useState<Capitalization>('CAPITALIZATION_MONTHLY');
  const r = depositSchedule({
    amount: majorToMinor(num(amount), cur),
    ratePct: num(rate),
    months: num(months),
    cap,
    topUp: majorToMinor(num(topUp), cur),
    taxPct: num(tax),
  });
  const fmt = (v: number) => formatMoney(Math.round(v), cur);
  return (
    <>
      <Card>
        <Pair>
          <Field style={styles.flex1} label={`Amount, ${cur}`} value={amount} onChangeText={setAmount} keyboardType="decimal-pad" />
          <Field style={styles.flex1} label="Rate, % / year" value={rate} onChangeText={setRate} keyboardType="decimal-pad" />
        </Pair>
        <Pair>
          <Field style={styles.flex1} label="Term, months" value={months} onChangeText={setMonths} keyboardType="number-pad" />
          <Field style={styles.flex1} label="Monthly top-up" value={topUp} onChangeText={setTopUp} keyboardType="decimal-pad" />
        </Pair>
        <Txt variant="label">Capitalization</Txt>
        <Segmented options={CAPITALIZATIONS.map((x) => ({ value: x.value, label: x.label }))} value={cap} onChange={setCap} />
        <Field label="Tax on interest, % (KZ deposits: 0)" value={tax} onChangeText={setTax} keyboardType="decimal-pad" />
      </Card>
      <Card>
        <Stat label="You put in" value={fmt(r.contributed)} />
        <Stat label="Interest earned" value={`+${fmt(r.interest)}`} color={c.positive} />
        {r.tax > 0 ? <Stat label="Tax" value={`−${fmt(r.tax)}`} color={c.negative} /> : null}
        <Stat label="At the end" value={fmt(r.net)} strong />
        <Stat label="Effective yield" value={`${r.effective.toFixed(2)}% / yr`} />
        {r.rows.length > 1 ? (
          <LineChart
            series={[
              { name: 'Balance', color: c.savings, values: r.rows.map((x) => x.value), area: true },
              { name: 'Put in', color: c.textMuted, values: r.rows.map((x) => x.contributed), dashed: true },
            ]}
            labels={r.rows.map((x) => String(x.month))}
            formatLabel={(l) => `Month ${l}`}
            formatValue={fmt}
            formatAxis={(v) => compactMoney(v, cur)}
            animateKey="deposit"
          />
        ) : null}
      </Card>
    </>
  );
}

function InvestCalc({ cur }: { cur: string }) {
  const { c } = useTheme();
  const [initial, setInitial] = useState('500000');
  const [monthly, setMonthly] = useState('50000');
  const [ret, setRet] = useState('10');
  const [years, setYears] = useState('15');
  const [infl, setInfl] = useState('8');
  const [fee, setFee] = useState('0.2');
  const [target, setTarget] = useState('');
  const r = compoundSchedule({
    initial: majorToMinor(num(initial), cur),
    monthly: majorToMinor(num(monthly), cur),
    returnPct: num(ret),
    years: num(years),
    inflationPct: num(infl),
    feePct: num(fee),
  });
  const fmt = (v: number) => formatMoney(Math.round(v), cur);
  const need = target.trim()
    ? monthlyForTarget(majorToMinor(num(target), cur), majorToMinor(num(initial), cur), num(ret) - num(fee), num(years))
    : null;
  return (
    <>
      <Card>
        <Pair>
          <Field style={styles.flex1} label={`Start with, ${cur}`} value={initial} onChangeText={setInitial} keyboardType="decimal-pad" />
          <Field style={styles.flex1} label="Every month" value={monthly} onChangeText={setMonthly} keyboardType="decimal-pad" />
        </Pair>
        <Pair>
          <Field style={styles.flex1} label="Return, % / year" value={ret} onChangeText={setRet} keyboardType="decimal-pad" />
          <Field style={styles.flex1} label="Years" value={years} onChangeText={setYears} keyboardType="number-pad" />
        </Pair>
        <Pair>
          <Field style={styles.flex1} label="Inflation, % / year" value={infl} onChangeText={setInfl} keyboardType="decimal-pad" />
          <Field style={styles.flex1} label="Fund fee, % / year" value={fee} onChangeText={setFee} keyboardType="decimal-pad" />
        </Pair>
        <Txt variant="small">S&P 500 has returned ~10%/yr in USD over the long run; past returns don&apos;t guarantee future ones.</Txt>
      </Card>
      <Card>
        <Stat label="You put in" value={fmt(r.contributed)} />
        <Stat label="Growth" value={`+${fmt(r.growth)}`} color={c.positive} />
        <Stat label="Total" value={fmt(r.final)} strong />
        <Stat label="In today's money" value={fmt(r.real)} />
        {r.rows.length > 1 ? (
          <LineChart
            series={[
              { name: 'Total', color: c.primary, values: r.rows.map((x) => x.value), area: true },
              { name: "Today's money", color: c.wants, values: r.rows.map((x) => x.real) },
              { name: 'Put in', color: c.textMuted, values: r.rows.map((x) => x.contributed), dashed: true },
            ]}
            labels={r.rows.map((x) => String(x.year))}
            formatLabel={(l) => `Year ${l}`}
            formatValue={fmt}
            formatAxis={(v) => compactMoney(v, cur)}
            ticks={Math.min(6, r.rows.length)}
            animateKey="invest"
          />
        ) : null}
      </Card>
      <SectionHeader title="Reach a target" />
      <Card>
        <Field label={`Target amount, ${cur}`} value={target} onChangeText={setTarget} keyboardType="decimal-pad" placeholder="50000000" />
        {need !== null ? (
          <Txt>
            Invest <Txt weight="800">{fmt(need)}</Txt> a month for {num(years)} years at {num(ret) - num(fee)}% net.
          </Txt>
        ) : (
          <Txt variant="small">Enter a target to see the monthly amount needed with the settings above.</Txt>
        )}
      </Card>
    </>
  );
}

function LoanCalc({ cur }: { cur: string }) {
  const { c } = useTheme();
  const [amount, setAmount] = useState('5000000');
  const [rate, setRate] = useState('22');
  const [months, setMonths] = useState('36');
  const [type, setType] = useState<LoanType>('annuity');
  const a = majorToMinor(num(amount), cur);
  const ann = loanSchedule(a, num(rate), num(months), 'annuity');
  const diff = loanSchedule(a, num(rate), num(months), 'differentiated');
  const r = type === 'annuity' ? ann : diff;
  const fmt = (v: number) => formatMoney(Math.round(v), cur);
  return (
    <>
      <Card>
        <Field label={`Loan amount, ${cur}`} value={amount} onChangeText={setAmount} keyboardType="decimal-pad" />
        <Pair>
          <Field style={styles.flex1} label="Rate, % / year" value={rate} onChangeText={setRate} keyboardType="decimal-pad" />
          <Field style={styles.flex1} label="Term, months" value={months} onChangeText={setMonths} keyboardType="number-pad" />
        </Pair>
        <Segmented
          options={[
            { value: 'annuity', label: 'Annuity (equal)' },
            { value: 'differentiated', label: 'Differentiated' },
          ]}
          value={type}
          onChange={setType}
        />
      </Card>
      <Card>
        <Stat label={type === 'annuity' ? 'Monthly payment' : 'First → last payment'} value={type === 'annuity' ? fmt(r.first) : `${fmt(r.first)} → ${fmt(r.last)}`} strong />
        <Stat label="Overpayment (interest)" value={fmt(r.interest)} color={c.negative} />
        <Stat label="Total paid" value={fmt(r.totalPaid)} />
        <Txt variant="small">
          Differentiated saves {fmt(Math.max(0, ann.interest - diff.interest))} in interest but starts with higher payments.
        </Txt>
        <LineChart
          series={[
            { name: 'Annuity', color: c.primary, values: ann.payments },
            { name: 'Differentiated', color: c.wants, values: diff.payments },
          ]}
          labels={ann.payments.map((_, i) => String(i + 1))}
          formatLabel={(l) => `Month ${l}`}
          formatValue={fmt}
          formatAxis={(v) => compactMoney(v, cur)}
          zeroBased={false}
          animateKey="loan"
        />
        <Txt variant="small">Monthly payment by month.</Txt>
      </Card>
    </>
  );
}

export default function CalculatorsScreen() {
  const { tab: initial } = useLocalSearchParams<{ tab?: Tab }>();
  const [tab, setTab] = useState<Tab>(initial ?? 'deposit');
  const { user } = useAuth();
  const cur = user?.currency || 'USD';
  return (
    <Screen>
      <Segmented
        options={[
          { value: 'deposit', label: 'Deposit', icon: 'business-outline' },
          { value: 'invest', label: 'Investing', icon: 'trending-up' },
          { value: 'loan', label: 'Loan', icon: 'card-outline' },
        ]}
        value={tab}
        onChange={setTab}
      />
      <View style={styles.body}>{tab === 'deposit' ? <DepositCalc cur={cur} /> : tab === 'invest' ? <InvestCalc cur={cur} /> : <LoanCalc cur={cur} />}</View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  body: { gap: spacing.md },
  stat: { paddingVertical: 2 },
  num: { fontVariant: ['tabular-nums'] },
});
