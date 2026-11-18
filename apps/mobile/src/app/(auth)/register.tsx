import { Link } from 'expo-router';
import React, { useState } from 'react';

import { errorMessage } from '@/api/client';
import { useAuth } from '@/auth/AuthContext';
import { AuthShell } from '@/components/AuthShell';
import { CurrencyField } from '@/components/CurrencyField';
import { Banner, Button, Field, Row, Txt } from '@/components/ui';
import { useTheme } from '@/theme';

function guessCurrency(): string {
  try {
    const locale = Intl.NumberFormat().resolvedOptions().locale;
    const region = locale.split('-')[1]?.toUpperCase();
    const map: Record<string, string> = {
      US: 'USD', GB: 'GBP', CA: 'CAD', AU: 'AUD', CH: 'CHF', JP: 'JPY', PL: 'PLN', SE: 'SEK', NO: 'NOK',
      CZ: 'CZK', TR: 'TRY', KZ: 'KZT', GE: 'GEL', RU: 'RUB', DE: 'EUR', FR: 'EUR', ES: 'EUR', IT: 'EUR',
      NL: 'EUR', AT: 'EUR', BE: 'EUR', PT: 'EUR', FI: 'EUR', IE: 'EUR',
    };
    return (region && map[region]) || 'USD';
  } catch {
    return 'USD';
  }
}

export default function RegisterScreen() {
  const { register } = useAuth();
  const { c } = useTheme();
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [currency, setCurrency] = useState(guessCurrency);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (!name.trim() || !email.trim() || !password) {
      setError('Fill in name, email and password');
      return;
    }
    if (password.length < 8) {
      setError('Password must be at least 8 characters');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await register({ name: name.trim(), email: email.trim(), password, currency });
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthShell title="Create account" subtitle="It takes less than a minute">
      {error ? <Banner kind="error" message={error} /> : null}
      <Field label="Name" value={name} onChangeText={setName} autoComplete="name" placeholder="Alex Smith" />
      <Field
        label="Email"
        value={email}
        onChangeText={setEmail}
        autoCapitalize="none"
        autoComplete="email"
        keyboardType="email-address"
        placeholder="you@example.com"
      />
      <Field
        label="Password"
        value={password}
        onChangeText={setPassword}
        secureTextEntry
        autoComplete="new-password"
        placeholder="At least 8 characters"
      />
      {/* ListCurrencies is public; falls back to a short built-in list if the server is unreachable. */}
      <CurrencyField label="Home currency" value={currency} onChange={setCurrency} />
      <Txt variant="small">You can change your home currency later in Settings.</Txt>
      <Button title="Create account" onPress={submit} loading={busy} />
      <Row style={{ justifyContent: 'center' }}>
        <Txt variant="muted">Already registered?</Txt>
        <Link href="/login" replace>
          <Txt variant="muted" color={c.primary} weight="700">
            Log in
          </Txt>
        </Link>
      </Row>
    </AuthShell>
  );
}
