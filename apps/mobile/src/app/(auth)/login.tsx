import { Link } from 'expo-router';
import React, { useState } from 'react';

import { errorMessage } from '@/api/client';
import { useAuth } from '@/auth/AuthContext';
import { AuthShell } from '@/components/AuthShell';
import { Banner, Button, Field, Row, Txt } from '@/components/ui';
import { useTheme } from '@/theme';

export default function LoginScreen() {
  const { login } = useAuth();
  const { c } = useTheme();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (!email.trim() || !password) {
      setError('Enter your email and password');
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await login({ email: email.trim(), password });
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthShell title="Welcome back" subtitle="Log in to your account">
      {error ? <Banner kind="error" message={error} /> : null}
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
        autoComplete="password"
        placeholder="••••••••"
        onSubmitEditing={submit}
      />
      <Button title="Log in" onPress={submit} loading={busy} />
      <Row style={{ justifyContent: 'center' }}>
        <Txt variant="muted">No account?</Txt>
        <Link href="/register" replace>
          <Txt variant="muted" color={c.primary} weight="700">
            Create one
          </Txt>
        </Link>
      </Row>
    </AuthShell>
  );
}
