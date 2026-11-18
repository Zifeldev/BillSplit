import { useMutation } from '@tanstack/react-query';
import { useLocalSearchParams } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { FinanceApi } from '@/api/services';
import type { ExportFormat, ExportKind } from '@/api/types';
import { Banner, Button, Card, IconButton, IconTile, Row, Screen, Txt } from '@/components/ui';
import { notify } from '@/lib/dialogs';
import { monthLabel } from '@/lib/format';
import { saveFile } from '@/lib/save-file';
import { spacing, useTheme } from '@/theme';

type Job = { kind: ExportKind; format: ExportFormat };

export default function ExportScreen() {
  const params = useLocalSearchParams<{ year?: string; month?: string }>();
  const { c } = useTheme();
  const now = new Date();
  const [ym, setYm] = useState({
    year: Number(params.year) || now.getFullYear(),
    month: Number(params.month) || now.getMonth() + 1,
  });
  const [error, setError] = useState<string | null>(null);
  const [running, setRunning] = useState<Job | null>(null);
  const future = ym.year > now.getFullYear() || (ym.year === now.getFullYear() && ym.month > now.getMonth() + 1);

  const run = useMutation({
    mutationFn: async (job: Job) => {
      setRunning(job);
      const f = await FinanceApi.exportReport({ ...job, ...(job.kind === 'EXPORT_KIND_MONTH' ? ym : {}) });
      await saveFile(f.filename, f.contentType, f.data);
      return f;
    },
    onSuccess: (f) => {
      setError(null);
      notify('Report ready', f.filename);
    },
    onError: (e) => setError(errorMessage(e)),
    onSettled: () => setRunning(null),
  });
  const busy = (kind: ExportKind, format: ExportFormat) => running?.kind === kind && running.format === format;
  const shift = (d: number) =>
    setYm((p) => {
      const dt = new Date(p.year, p.month - 1 + d, 1);
      return { year: dt.getFullYear(), month: dt.getMonth() + 1 };
    });

  return (
    <Screen>
      {error ? <Banner kind="error" message={error} /> : null}
      <Card>
        <Row gap={spacing.md}>
          <IconTile icon="calendar" color={c.primary} size={44} />
          <View style={styles.flex1}>
            <Txt variant="h3">Month report</Txt>
            <Txt variant="small">Income, spending by category, daily chart, top merchants and every transaction.</Txt>
          </View>
        </Row>
        <Row style={[styles.month, { backgroundColor: c.cardAlt }]}>
          <IconButton icon="chevron-back" onPress={() => shift(-1)} label="Previous month" />
          <Txt weight="700" style={styles.monthLabel}>
            {monthLabel(ym.year, ym.month)}
          </Txt>
          <IconButton icon="chevron-forward" onPress={() => shift(1)} label="Next month" />
        </Row>
        <Row gap={spacing.sm}>
          <Button
            title="PDF"
            icon="document-text-outline"
            style={styles.flex1}
            disabled={future || run.isPending}
            loading={busy('EXPORT_KIND_MONTH', 'EXPORT_FORMAT_PDF')}
            onPress={() => run.mutate({ kind: 'EXPORT_KIND_MONTH', format: 'EXPORT_FORMAT_PDF' })}
          />
          <Button
            title="CSV (Excel)"
            icon="grid-outline"
            variant="secondary"
            style={styles.flex1}
            disabled={future || run.isPending}
            loading={busy('EXPORT_KIND_MONTH', 'EXPORT_FORMAT_CSV')}
            onPress={() => run.mutate({ kind: 'EXPORT_KIND_MONTH', format: 'EXPORT_FORMAT_CSV' })}
          />
        </Row>
      </Card>

      <Card>
        <Row gap={spacing.md}>
          <IconTile icon="pie-chart" color={c.positive} size={44} />
          <View style={styles.flex1}>
            <Txt variant="h3">Net worth & portfolio</Txt>
            <Txt variant="small">Today&apos;s net worth, its history, investments with gains and all accounts.</Txt>
          </View>
        </Row>
        <Row gap={spacing.sm}>
          <Button
            title="PDF"
            icon="document-text-outline"
            style={styles.flex1}
            disabled={run.isPending}
            loading={busy('EXPORT_KIND_PORTFOLIO', 'EXPORT_FORMAT_PDF')}
            onPress={() => run.mutate({ kind: 'EXPORT_KIND_PORTFOLIO', format: 'EXPORT_FORMAT_PDF' })}
          />
          <Button
            title="CSV (Excel)"
            icon="grid-outline"
            variant="secondary"
            style={styles.flex1}
            disabled={run.isPending}
            loading={busy('EXPORT_KIND_PORTFOLIO', 'EXPORT_FORMAT_CSV')}
            onPress={() => run.mutate({ kind: 'EXPORT_KIND_PORTFOLIO', format: 'EXPORT_FORMAT_CSV' })}
          />
        </Row>
      </Card>
      <Txt variant="small" style={styles.note}>
        Files are generated on your server and saved to this device — nothing is sent anywhere else.
      </Txt>
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  month: { borderRadius: 12, paddingHorizontal: spacing.xs },
  monthLabel: { flex: 1, textAlign: 'center' },
  note: { textAlign: 'center' },
});
