import { Icon } from '@/components/Icon';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import * as DocumentPicker from 'expo-document-picker';
import { useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { importRunning, qk, useStatementImport, useStatementImports } from '@/api/queries';
import { FinanceApi } from '@/api/services';
import type { StatementFormat, StatementImport } from '@/api/types';
import { useAuth } from '@/auth/AuthContext';
import { CurrencyField } from '@/components/CurrencyField';
import { Celebration } from '@/components/Celebration';
import {
  Badge,
  Banner,
  Button,
  Card,
  EmptyView,
  ErrorView,
  Field,
  IconTile,
  ListRow,
  Row,
  Screen,
  SectionHeader,
  Segmented,
  SkeletonList,
  Txt,
} from '@/components/ui';
import { formatDate } from '@/lib/format';
import { formatBytes, readPickedFile } from '@/lib/picked-file';
import { radius, spacing, useTheme } from '@/theme';

type Fmt = 'auto' | 'csv' | 'ofx' | 'pdf';
const FORMAT: Record<Fmt, StatementFormat> = {
  auto: 'STATEMENT_FORMAT_UNSPECIFIED',
  csv: 'STATEMENT_FORMAT_CSV',
  ofx: 'STATEMENT_FORMAT_OFX',
  pdf: 'STATEMENT_FORMAT_PDF',
};
const MAX_BYTES = 15 * 1024 * 1024;

function ImportRow({ imp }: { imp: StatementImport }) {
  const { c } = useTheme();
  const running = importRunning(imp);
  const failed = imp.status === 'IMPORT_STATUS_FAILED';
  return (
    <ListRow
      left={
        running ? (
          <ActivityIndicator color={c.primary} />
        ) : (
          <Icon
            name={failed ? 'close-circle' : 'checkmark-circle'}
            size={24}
            color={failed ? c.danger : c.primary}
          />
        )
      }
      title={formatDate(imp.createdAt) || 'Import'}
      subtitle={
        running
          ? 'Processing…'
          : failed
            ? imp.error || 'Failed'
            : `${imp.imported} imported · ${imp.duplicates} duplicates skipped`
      }
    />
  );
}

export default function ImportStatementScreen() {
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const { user } = useAuth();
  const [file, setFile] = useState<{ name: string; size?: number; base64: string; isPdf: boolean } | null>(null);
  const [dismissed, setDismissed] = useState<string | null>(null);
  const [fmt, setFmt] = useState<Fmt>('auto');
  const [currency, setCurrency] = useState('');
  const [bank, setBank] = useState('');
  const [importId, setImportId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const status = useStatementImport(importId);
  const s = status.data;
  const done = s?.status === 'IMPORT_STATUS_DONE';
  const failed = s?.status === 'IMPORT_STATUS_FAILED';
  const history = useStatementImports();

  useEffect(() => {
    if (done || failed) void qc.invalidateQueries({ queryKey: qk.statementImports });
    if (done) {
      void qc.invalidateQueries({ queryKey: qk.transactionsAll });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
      void qc.invalidateQueries({ queryKey: qk.recurring });
    }
  }, [done, failed, qc]);

  const pick = async () => {
    setError(null);
    try {
      const res = await DocumentPicker.getDocumentAsync({
        type: [
          'application/pdf',
          'text/csv',
          'text/comma-separated-values',
          'application/x-ofx',
          'application/vnd.intu.qfx',
          'text/plain',
          '*/*',
        ],
        copyToCacheDirectory: true,
        multiple: false,
        base64: true,
      });
      if (res.canceled || !res.assets?.[0]) return;
      const f = await readPickedFile(res.assets[0], MAX_BYTES);
      setFile({ name: f.name, size: f.size, base64: f.base64, isPdf: f.isPdf });
      const lower = f.name.toLowerCase();
      if (f.isPdf) setFmt('pdf');
      else if (lower.endsWith('.csv')) setFmt('csv');
      else if (lower.endsWith('.ofx') || lower.endsWith('.qfx')) setFmt('ofx');
    } catch (e) {
      setError(errorMessage(e));
    }
  };

  const start = useMutation({
    mutationFn: () => {
      if (!file) throw new Error('Choose a file first');
      return FinanceApi.importStatement({
        file: file.base64,
        format: FORMAT[fmt],
        bank: bank.trim() || undefined,
        currency: currency || undefined,
      });
    },
    onSuccess: (imp) => {
      qc.setQueryData(qk.statementImport(imp.id), imp);
      setImportId(imp.id);
      void qc.invalidateQueries({ queryKey: qk.statementImports });
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const reset = () => {
    setImportId(null);
    setFile(null);
    setError(null);
  };

  if (importId) {
    return (
      <Screen>
        <Card style={styles.statusCard}>
          {!s || (!done && !failed) ? (
            <>
              <ActivityIndicator size="large" color={c.primary} />
              <Txt variant="h3" style={styles.center}>
                Importing {file?.name}…
              </Txt>
              <Txt variant="muted" style={styles.center}>
                {file?.isPdf ? 'PDF statements take a little longer to read. ' : ''}Checking every 2 seconds.
              </Txt>
            </>
          ) : done ? (
            <>
              <IconTile icon="checkmark-done-circle" color={c.positive} size={64} />
              <Txt variant="h2">Import complete</Txt>
              <Row gap={spacing.xl}>
                <View style={styles.stat}>
                  <Txt variant="big">{s.imported}</Txt>
                  <Txt variant="muted">imported</Txt>
                </View>
                <View style={styles.stat}>
                  <Txt variant="big">{s.duplicates}</Txt>
                  <Txt variant="muted">duplicates skipped</Txt>
                </View>
              </Row>
            </>
          ) : (
            <>
              <Icon name="close-circle" size={48} color={c.danger} />
              <Txt variant="h2">Import failed</Txt>
              <Txt variant="muted" style={styles.center}>
                {s.error || 'The file could not be processed.'}
              </Txt>
            </>
          )}
          {status.error ? <Banner kind="error" message={errorMessage(status.error)} /> : null}
        </Card>
        {done ? <Button title="View transactions" icon="list-outline" onPress={() => router.replace('/finance/transactions')} /> : null}
        {done || failed || status.error ? <Button title="Import another file" variant="secondary" onPress={reset} /> : null}
        <Celebration
          visible={done && dismissed !== importId}
          icon="cloud-upload"
          title="Statement imported"
          message={s ? `${s.imported} transactions added, ${s.duplicates} duplicates skipped.` : undefined}
          onClose={() => setDismissed(importId)}
        />
      </Screen>
    );
  }

  return (
    <Screen>
      {error ? <Banner kind="error" message={error} /> : null}
      <Card>
        <Row gap={spacing.md}>
          <IconTile icon="document-text" color={c.primary} size={44} />
          <View style={styles.flex1}>
            <Txt variant="h3">Bring in your bank history</Txt>
            <Txt variant="small">CSV, OFX/QFX or PDF statement. Duplicates are skipped automatically.</Txt>
          </View>
        </Row>
        <Row gap={6} style={styles.wrap}>
          <Badge label="Freedom Bank (Kazakhstan) PDF/CSV supported" tone="ok" icon="checkmark-circle" />
          <Badge label="Multi-currency" tone="brand" icon="globe" />
        </Row>
        <Pressable
          accessibilityRole="button"
          onPress={() => void pick()}
          style={({ pressed }) => [
            styles.fileBox,
            { borderColor: file ? c.primary : c.border, backgroundColor: file ? c.primarySoft : c.cardAlt },
            pressed && { opacity: 0.8 },
          ]}>
          <View style={[styles.fileIcon, { backgroundColor: file?.isPdf ? c.dangerSoft : c.card }]}>
            <Icon
              name={file ? (file.isPdf ? 'document-text' : 'document') : 'cloud-upload-outline'}
              size={26}
              color={file ? (file.isPdf ? c.danger : c.primary) : c.textMuted}
            />
          </View>
          <View style={styles.flex1}>
            <Row gap={6}>
              <Txt weight="700" numberOfLines={1} style={styles.shrink}>
                {file ? file.name : 'Choose a statement file'}
              </Txt>
              {file?.isPdf ? <Badge label="PDF" tone="bad" /> : null}
            </Row>
            <Txt variant="small">{file?.size ? formatBytes(file.size) : 'PDF up to 15 MB · CSV · OFX'}</Txt>
          </View>
          <Txt variant="small" weight="700" color={c.primary}>
            {file ? 'Change' : 'Browse'}
          </Txt>
        </Pressable>
        <Txt variant="label">Format</Txt>
        <Segmented
          options={[
            { value: 'auto', label: 'Auto' },
            { value: 'csv', label: 'CSV' },
            { value: 'ofx', label: 'OFX' },
            { value: 'pdf', label: 'PDF' },
          ]}
          value={fmt}
          onChange={setFmt}
        />
        <CurrencyField
          label="Default currency"
          value={currency}
          onChange={setCurrency}
          allowEmpty
          emptyLabel={`Home currency (${user?.currency ?? 'USD'})`}
        />
        <Txt variant="small">
          Used only for rows without their own currency. Multi-currency statements (e.g. Freedom Bank KZT/USD/EUR) keep
          each row&apos;s currency and are converted to your home currency in reports.
        </Txt>
        <Field label="Bank (optional hint)" value={bank} onChangeText={setBank} placeholder="freedom, chase, bofa…" autoCapitalize="none" />
      </Card>
      <Button title="Import" icon="cloud-upload-outline" disabled={!file} loading={start.isPending} onPress={() => start.mutate()} />

      <SectionHeader title="Recent imports" />
      {history.data ? (
        (history.data.imports ?? []).length === 0 ? (
          <Card>
            <EmptyView icon="document-text" title="No imports yet" message="Your imported statements will show up here." />
          </Card>
        ) : (
          <Card>
            {(history.data.imports ?? []).map((imp) => (
              <ImportRow key={imp.id} imp={imp} />
            ))}
          </Card>
        )
      ) : history.error ? (
        <ErrorView error={history.error} onRetry={() => void history.refetch()} />
      ) : (
        <SkeletonList rows={2} />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  center: { textAlign: 'center' },
  statusCard: { alignItems: 'center', paddingVertical: spacing.xxl, gap: spacing.md },
  stat: { alignItems: 'center' },
  wrap: { flexWrap: 'wrap' },
  shrink: { flexShrink: 1 },
  fileIcon: { width: 48, height: 48, borderRadius: 14, alignItems: 'center', justifyContent: 'center' },
  fileBox: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    padding: spacing.md,
    borderRadius: radius.md,
    borderWidth: 1.5,
    borderStyle: 'dashed',
    minHeight: 72,
  },
});
