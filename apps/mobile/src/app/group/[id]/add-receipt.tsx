import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Icon } from '@/components/Icon';
import * as DocumentPicker from 'expo-document-picker';
import { Image } from 'expo-image';
import * as ImagePicker from 'expo-image-picker';
import { Stack, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { StyleSheet, View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useGroup } from '@/api/queries';
import { ReceiptApi } from '@/api/services';
import type { ItemInput } from '@/api/types';
import { CurrencyField } from '@/components/CurrencyField';
import { Badge, Banner, Button, Card, Field, IconButton, Row, Screen, SectionHeader, Segmented, Txt } from '@/components/ui';
import { amountPlaceholder, parseMinor } from '@/lib/format';
import { base64Bytes, formatBytes, readPickedFile } from '@/lib/picked-file';
import { radius, spacing, useTheme } from '@/theme';

const MAX_BYTES = 8 * 1024 * 1024;

interface DraftItem {
  key: string;
  name: string;
  qty: string;
  price: string;
}

let keySeq = 0;
const newItem = (): DraftItem => ({ key: String(++keySeq), name: '', qty: '1', price: '' });

export default function AddReceiptScreen() {
  const params = useLocalSearchParams<{ id: string; mode?: string }>();
  const groupId = String(params.id ?? '');
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const group = useGroup(groupId);

  const [mode, setMode] = useState<'scan' | 'manual'>(params.mode === 'manual' ? 'manual' : 'scan');
  const [image, setImage] = useState<{
    uri: string;
    base64: string;
    isPdf: boolean;
    name: string;
    size: number;
  } | null>(null);
  const [currency, setCurrency] = useState('');
  const [tip, setTip] = useState('');
  const [tax, setTax] = useState('');
  const [store, setStore] = useState('');
  const [items, setItems] = useState<DraftItem[]>([newItem()]);
  const [error, setError] = useState<string | null>(null);
  const [stage, setStage] = useState<string | null>(null);

  const pick = async (source: 'camera' | 'library') => {
    setError(null);
    try {
      const opts: ImagePicker.ImagePickerOptions = { mediaTypes: ['images'], quality: 0.6, base64: true };
      if (source === 'camera') {
        const perm = await ImagePicker.requestCameraPermissionsAsync();
        if (!perm.granted) {
          setError('Camera permission is required to scan receipts.');
          return;
        }
      }
      const res =
        source === 'camera' ? await ImagePicker.launchCameraAsync(opts) : await ImagePicker.launchImageLibraryAsync(opts);
      if (res.canceled || !res.assets?.[0]) return;
      const asset = res.assets[0];
      let b64 = asset.base64 ?? '';
      if (!b64 && asset.uri.startsWith('data:')) b64 = asset.uri.slice(asset.uri.indexOf(',') + 1);
      if (!b64) {
        setError('Could not read the image.');
        return;
      }
      const size = base64Bytes(b64);
      if (size > MAX_BYTES) {
        setError(`This image is ${formatBytes(size)} — the limit is 8 MB. Try a lower resolution photo.`);
        return;
      }
      setImage({ uri: asset.uri, base64: b64, isPdf: false, name: asset.fileName || 'Photo', size });
    } catch (e) {
      setError(errorMessage(e));
    }
  };

  // PDF receipts (e-receipts, invoices) or image files from Files / the desktop.
  const pickFile = async () => {
    setError(null);
    try {
      const res = await DocumentPicker.getDocumentAsync({
        type: ['application/pdf', 'image/*'],
        copyToCacheDirectory: true,
        multiple: false,
        base64: true,
      });
      if (res.canceled || !res.assets?.[0]) return;
      const f = await readPickedFile(res.assets[0], MAX_BYTES);
      setImage({ uri: f.uri, base64: f.base64, isPdf: f.isPdf, name: f.name, size: f.size });
    } catch (e) {
      setError(errorMessage(e));
    }
  };

  // Amounts are typed in the receipt currency (group currency when "default").
  const cur = currency || group.data?.currency || 'USD';
  const tipCents = parseMinor(tip, cur) ?? 0;

  const manualItems = (): ItemInput[] | string => {
    const out: ItemInput[] = [];
    for (const it of items) {
      if (!it.name.trim() && !it.price.trim()) continue;
      const price = parseMinor(it.price, cur);
      const qty = Number.parseInt(it.qty || '1', 10);
      if (!it.name.trim()) return 'Every item needs a name';
      if (price === null || price < 0) return `Invalid price for "${it.name}"`;
      if (!Number.isFinite(qty) || qty < 1) return `Invalid quantity for "${it.name}"`;
      out.push({ name: it.name.trim(), quantity: qty, priceCents: price });
    }
    if (out.length === 0) return 'Add at least one item';
    return out;
  };

  const upload = useMutation({
    mutationFn: async () => {
      if (tip.trim() && parseMinor(tip, cur) === null) throw new Error('Invalid tip');
      if (mode === 'scan') {
        if (!image) throw new Error('Choose a photo or PDF first');
        setStage('Uploading…');
        return ReceiptApi.upload({ groupId, image: image.base64, currency, tipCents });
      }
      const list = manualItems();
      if (typeof list === 'string') throw new Error(list);
      const taxCents = parseMinor(tax, cur);
      if (tax.trim() && taxCents === null) throw new Error('Invalid tax');
      setStage('Creating receipt…');
      return ReceiptApi.create({
        groupId,
        store: store.trim(),
        currency,
        items: list,
        taxCents: taxCents ?? 0,
        tipCents,
      });
    },
    onSuccess: (receipt) => {
      setStage(null);
      qc.setQueryData(qk.receipt(receipt.id), receipt);
      void qc.invalidateQueries({ queryKey: qk.receipts(groupId) });
      router.replace(`/receipt/${receipt.id}`);
    },
    onError: (e) => {
      setStage(null);
      setError(errorMessage(e));
    },
  });

  const updateItem = (key: string, patch: Partial<DraftItem>) =>
    setItems((prev) => prev.map((it) => (it.key === key ? { ...it, ...patch } : it)));

  return (
    <Screen>
      <Stack.Screen options={{ title: group.data ? `Add receipt · ${group.data.name}` : 'Add receipt' }} />
      <Segmented
        options={[
          { value: 'scan', label: 'Scan', icon: 'scan' },
          { value: 'manual', label: 'Enter manually', icon: 'create' },
        ]}
        value={mode}
        onChange={setMode}
      />
      {error ? <Banner kind="error" message={error} /> : null}

      {mode === 'scan' ? (
        <Card>
          {image ? (
            image.isPdf ? (
              <View style={[styles.preview, styles.placeholder, { backgroundColor: c.cardAlt, borderColor: c.primary }]}>
                <View style={[styles.pdfIcon, { backgroundColor: c.dangerSoft }]}>
                  <Icon name="document-text" size={40} color={c.danger} />
                </View>
                <View>
                  <Badge label="PDF" tone="bad" icon="document" />
                </View>
                <Txt weight="700" numberOfLines={1} style={styles.fileName}>
                  {image.name}
                </Txt>
                <Txt variant="small">{formatBytes(image.size)} · ready to upload</Txt>
              </View>
            ) : (
              <View>
                <Image source={{ uri: image.uri }} style={[styles.preview, { backgroundColor: c.cardAlt }]} contentFit="contain" />
                <View style={styles.previewBadge}>
                  <Badge label={`${formatBytes(image.size)}`} tone="brand" icon="image" />
                </View>
              </View>
            )
          ) : (
            <View style={[styles.preview, styles.placeholder, { backgroundColor: c.cardAlt, borderColor: c.border }]}>
              <Icon name="camera" size={44} color={c.primary} weight="duotone" />
              <Txt weight="700">Snap the whole receipt</Txt>
              <Txt variant="small" style={styles.centerText}>
                Flat, well lit, all four corners visible. E-receipts and invoices work as PDF (up to 8 MB).
              </Txt>
            </View>
          )}
          <Row gap={spacing.sm}>
            <Button title="Camera" icon="camera" style={styles.flex1} onPress={() => void pick('camera')} />
            <Button
              title="Gallery"
              icon="images"
              variant="secondary"
              style={styles.flex1}
              onPress={() => void pick('library')}
            />
          </Row>
          <Button title="PDF / File" icon="document-attach" variant="ghost" onPress={() => void pickFile()} />
        </Card>
      ) : (
        <>
          <Card>
            <Field label="Store" value={store} onChangeText={setStore} placeholder="Store or restaurant" />
          </Card>
          <SectionHeader title="Items" action="+ Add item" onAction={() => setItems((p) => [...p, newItem()])} />
          <Card>
            {items.map((it) => (
              <Row key={it.key} gap={6} style={styles.itemRow}>
                <Field style={styles.flex3} placeholder="Item" value={it.name} onChangeText={(v) => updateItem(it.key, { name: v })} />
                <Field
                  style={styles.qty}
                  placeholder="Qty"
                  keyboardType="number-pad"
                  value={it.qty}
                  onChangeText={(v) => updateItem(it.key, { qty: v.replace(/\D/g, '') })}
                />
                <Field
                  style={styles.flex1}
                  placeholder={amountPlaceholder(cur)}
                  keyboardType="decimal-pad"
                  value={it.price}
                  onChangeText={(v) => updateItem(it.key, { price: v })}
                />
                <IconButton
                  icon="trash-outline"
                  color={c.danger}
                  label="Remove item"
                  onPress={() => setItems((p) => (p.length > 1 ? p.filter((x) => x.key !== it.key) : [newItem()]))}
                />
              </Row>
            ))}
            <Txt variant="small">Price is the line total (quantity × unit price).</Txt>
          </Card>
          <Card>
            <Field label="Tax" value={tax} onChangeText={setTax} keyboardType="decimal-pad" placeholder={amountPlaceholder(cur)} />
          </Card>
        </>
      )}

      <Card>
        <CurrencyField
          label="Receipt currency"
          value={currency}
          onChange={setCurrency}
          allowEmpty
          emptyLabel={`Group currency${group.data ? ` (${group.data.currency})` : ''}`}
        />
        <Field
          label="Tip (optional)"
          value={tip}
          onChangeText={setTip}
          keyboardType="decimal-pad"
          placeholder={amountPlaceholder(cur)}
        />
      </Card>

      <Button
        title={stage ?? (mode === 'scan' ? (image?.isPdf ? 'Upload PDF receipt' : 'Upload receipt') : 'Create receipt')}
        icon="cloud-upload-outline"
        loading={upload.isPending}
        disabled={mode === 'scan' && !image}
        onPress={() => {
          setError(null);
          upload.mutate();
        }}
      />
      {stage ? (
        <Txt variant="small" style={{ textAlign: 'center' }}>
          {stage}
        </Txt>
      ) : null}
    </Screen>
  );
}

const styles = StyleSheet.create({
  flex1: { flex: 1 },
  flex3: { flex: 3 },
  qty: { width: 56 },
  itemRow: { alignItems: 'center' },
  preview: { width: '100%', height: 260, borderRadius: radius.md },
  placeholder: {
    alignItems: 'center',
    justifyContent: 'center',
    borderWidth: 1.5,
    borderStyle: 'dashed',
    gap: spacing.sm,
    padding: spacing.lg,
  },
  centerText: { textAlign: 'center', maxWidth: 320 },
  pdfIcon: { width: 76, height: 76, borderRadius: 22, alignItems: 'center', justifyContent: 'center' },
  fileName: { maxWidth: '90%' },
  previewBadge: { position: 'absolute', top: spacing.sm, right: spacing.sm },
});
