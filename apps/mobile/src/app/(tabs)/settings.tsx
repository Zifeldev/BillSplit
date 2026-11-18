import { useMutation, useQueryClient } from '@tanstack/react-query';
import Constants from 'expo-constants';
import React, { useState } from 'react';
import { View } from 'react-native';

import { API_URL, errorMessage } from '@/api/client';
import { qk } from '@/api/queries';
import { AuthApi } from '@/api/services';
import { useAuth } from '@/auth/AuthContext';
import { PaletteGrid } from '@/components/Appearance';
import { CurrencyField } from '@/components/CurrencyField';
import {
  Avatar,
  Banner,
  Button,
  Card,
  Field,
  HeroCard,
  IconName,
  IconTile,
  Row,
  Screen,
  SectionHeader,
  Segmented,
  Txt,
} from '@/components/ui';
import { confirm } from '@/lib/dialogs';
import { setThemePreference, useTheme, useThemePreference, type ThemePreference } from '@/theme';

const THEME_OPTIONS: { value: ThemePreference; label: string; icon: IconName }[] = [
  { value: 'system', label: 'System', icon: 'contrast' },
  { value: 'light', label: 'Light', icon: 'sun' },
  { value: 'dark', label: 'Dark', icon: 'moon' },
];

export default function SettingsScreen() {
  const { user, setUser, logout } = useAuth();
  const { c } = useTheme();
  const themePreference = useThemePreference();
  const qc = useQueryClient();
  const [name, setName] = useState(user?.name ?? '');
  const [currency, setCurrency] = useState(user?.currency ?? 'USD');
  const [venmo, setVenmo] = useState(user?.payment?.venmo ?? '');
  const [paypal, setPaypal] = useState(user?.payment?.paypal ?? '');
  const [zelle, setZelle] = useState(user?.payment?.zelle ?? '');
  const [payMsg, setPayMsg] = useState<{ kind: 'error' | 'success'; text: string } | null>(null);
  const handle = (s: string) => s.trim().replace(/^@/, '');
  const [msg, setMsg] = useState<{ kind: 'error' | 'success'; text: string } | null>(null);

  const save = useMutation({
    mutationFn: () =>
      AuthApi.updateProfile({
        name: name.trim() !== user?.name ? name.trim() : '',
        currency: currency !== user?.currency ? currency : '',
      }),
    onSuccess: (u) => {
      setUser(u);
      setMsg({ kind: 'success', text: 'Profile saved' });
      // Home currency affects every converted amount.
      void qc.invalidateQueries({ queryKey: qk.myBalances });
      void qc.invalidateQueries({ queryKey: qk.dashboardAll });
      void qc.invalidateQueries({ queryKey: qk.goals });
      void qc.invalidateQueries({ queryKey: qk.groups });
    },
    onError: (e) => setMsg({ kind: 'error', text: errorMessage(e) }),
  });

  const dirty = name.trim() !== (user?.name ?? '') || currency !== (user?.currency ?? '');

  const cur = user?.payment ?? { venmo: '', paypal: '', zelle: '' };
  const payDirty =
    handle(venmo) !== (cur.venmo ?? '') || handle(paypal) !== (cur.paypal ?? '') || zelle.trim() !== (cur.zelle ?? '');

  // Only changed handles are sent; "" clears a handle.
  const savePayment = useMutation({
    mutationFn: () =>
      AuthApi.updateProfile({
        name: '',
        currency: '',
        ...(handle(venmo) !== (cur.venmo ?? '') ? { venmo: handle(venmo) } : {}),
        ...(handle(paypal) !== (cur.paypal ?? '') ? { paypal: handle(paypal) } : {}),
        ...(zelle.trim() !== (cur.zelle ?? '') ? { zelle: zelle.trim() } : {}),
      }),
    onSuccess: (u) => {
      setUser(u);
      setVenmo(u.payment?.venmo ?? '');
      setPaypal(u.payment?.paypal ?? '');
      setZelle(u.payment?.zelle ?? '');
      setPayMsg({ kind: 'success', text: 'Payment details saved' });
    },
    onError: (e) => setPayMsg({ kind: 'error', text: errorMessage(e) }),
  });

  return (
    <Screen>
      <HeroCard>
        <Row gap={14}>
          <Avatar name={user?.name} id={user?.id} size={64} ring="rgba(255,255,255,0.9)" />
          <View style={{ flex: 1 }}>
            <Txt variant="title" color="#FFFFFF" numberOfLines={1}>
              {user?.name || 'Your profile'}
            </Txt>
            <Txt variant="muted" color="rgba(255,255,255,0.85)" numberOfLines={1}>
              {user?.email}
            </Txt>
            <Txt variant="small" color="rgba(255,255,255,0.85)" weight="700">
              Home currency · {user?.currency ?? 'USD'}
            </Txt>
          </View>
        </Row>
      </HeroCard>

      {msg ? <Banner kind={msg.kind} message={msg.text} /> : null}

      <SectionHeader title="Profile" />
      <Card>
        <Field label="Name" value={name} onChangeText={setName} />
        <CurrencyField label="Home currency" value={currency} onChange={setCurrency} />
        <Txt variant="small">Dashboards and balances across groups are converted to your home currency.</Txt>
        <Button title="Save" icon="checkmark" disabled={!dirty || !name.trim()} loading={save.isPending} onPress={() => save.mutate()} />
      </Card>

      <SectionHeader title="Appearance" />
      <Card>
        <Segmented options={THEME_OPTIONS} value={themePreference} onChange={setThemePreference} />
        <Txt variant="small">System follows the light or dark setting of your device.</Txt>
        <Txt variant="label">Color palette</Txt>
        <PaletteGrid />
      </Card>

      <SectionHeader title="Get paid back" />
      <Card>
        <Row gap={10}>
          <IconTile icon="card" color={c.primary} size={40} />
          <Txt weight="700" style={{ flex: 1 }}>
            Venmo · PayPal · Zelle
          </Txt>
        </Row>
        <Txt variant="small">
          Friends who owe you see “Pay with Venmo / PayPal / Zelle” buttons. Leave a field empty to remove it.
        </Txt>
        {payMsg ? <Banner kind={payMsg.kind} message={payMsg.text} /> : null}
        <Field label="Venmo username" value={venmo} onChangeText={setVenmo} autoCapitalize="none" autoCorrect={false} placeholder="alex-smith" />
        <Field label="PayPal.me username" value={paypal} onChangeText={setPaypal} autoCapitalize="none" autoCorrect={false} placeholder="alexsmith" />
        <Field
          label="Zelle email or phone"
          value={zelle}
          onChangeText={setZelle}
          autoCapitalize="none"
          autoCorrect={false}
          keyboardType="email-address"
          placeholder="alex@example.com"
        />
        <Button
          title="Save payment details"
          icon="checkmark"
          variant="secondary"
          disabled={!payDirty}
          loading={savePayment.isPending}
          onPress={() => {
            setPayMsg(null);
            savePayment.mutate();
          }}
        />
      </Card>

      <SectionHeader title="About" />
      <Card>
        <Row gap={10}>
          <IconTile icon="information" color={c.primary} size={40} />
          <View style={{ flex: 1 }}>
            <Txt weight="700">Billsplit {Constants.expoConfig?.version ?? '1.0.0'}</Txt>
            <Txt variant="small" numberOfLines={1}>
              Server: {API_URL}
            </Txt>
          </View>
        </Row>
      </Card>

      <Button
        title="Log out"
        icon="log-out-outline"
        variant="danger"
        onPress={async () => {
          const ok = await confirm('Log out', 'Do you want to log out?', 'Log out', true);
          if (ok) await logout();
        }}
      />
    </Screen>
  );
}
