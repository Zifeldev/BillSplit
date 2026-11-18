import { QueryClient, QueryClientProvider, useQueryClient } from '@tanstack/react-query';
import { useFonts } from 'expo-font';
import { DarkTheme, DefaultTheme, Stack, ThemeProvider, useRouter } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { StatusBar } from 'expo-status-bar';
import React, { useEffect, useRef, useState } from 'react';
import { StyleSheet, View } from 'react-native';
import { SafeAreaProvider } from 'react-native-safe-area-context';

import { ApiError } from '@/api/client';
import { pendingInvite } from '@/api/tokens';
import { AuthProvider, useAuth } from '@/auth/AuthContext';
import { Backdrop, HAS_LIQUID_GLASS, HeaderGlass } from '@/components/Glass';
import { Sidebar } from '@/components/Sidebar';
import { LoadingView } from '@/components/ui';
import { notificationHref } from '@/lib/notification-route';
import { addPushResponseListener, consumeInitialPushData } from '@/lib/push';
import { fontAssets, fonts, loadThemePreference, useIsWide, useTheme } from '@/theme';

void SplashScreen.preventAutoHideAsync().catch(() => undefined);

function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 15_000,
        retry: (count, err) => {
          if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
          return count < 2;
        },
      },
      mutations: { retry: false },
    },
  });
}

function renderHeaderGlass() {
  return <HeaderGlass />;
}

function RootNavigator() {
  const { status } = useAuth();
  const { c, dark } = useTheme();
  const wide = useIsWide();
  const router = useRouter();
  const prevStatus = useRef(status);
  const qc = useQueryClient();

  useEffect(() => {
    if (status !== 'loading') void SplashScreen.hideAsync().catch(() => undefined);
    // After a fresh login/registration, continue a pending invite.
    if (prevStatus.current === 'guest' && status === 'authed') {
      void pendingInvite.get().then((token) => {
        // Let the navigator apply the new guards first.
        if (token) setTimeout(() => router.replace(`/invite/${encodeURIComponent(token)}`), 0);
      });
    }
    prevStatus.current = status;
  }, [status, router]);

  // Tapping a push notification navigates like tapping it in the inbox.
  const authed = status === 'authed';
  useEffect(() => {
    if (!authed) return;
    const go = (data: Record<string, unknown>) => {
      void qc.invalidateQueries({ queryKey: ['notifications'] });
      router.push(notificationHref(data));
    };
    void consumeInitialPushData().then((data) => {
      if (data) setTimeout(() => go(data), 0);
    });
    return addPushResponseListener(go);
  }, [authed, router, qc]);

  const navTheme = dark
    ? { ...DarkTheme, colors: { ...DarkTheme.colors, background: c.bg, card: c.card, primary: c.primary, text: c.text, border: c.border } }
    : { ...DefaultTheme, colors: { ...DefaultTheme.colors, background: c.bg, card: c.card, primary: c.primary, text: c.text, border: c.border } };

  if (status === 'loading') return <LoadingView />;

  return (
    <ThemeProvider value={navTheme}>
      <StatusBar style={dark ? 'light' : 'dark'} />
      <View style={styles.shell}>
        <Backdrop />
        {authed && wide ? <Sidebar /> : null}
        <View style={styles.fill}>
          <Stack
            screenOptions={{
              headerTransparent: true,
              // iOS 26 draws its own Liquid Glass bar; everything else gets the frosted fallback.
              headerBackground: HAS_LIQUID_GLASS ? undefined : renderHeaderGlass,
              headerTintColor: c.text,
              headerTitleStyle: { fontFamily: fonts.display, fontSize: 19 },
              headerBackTitleStyle: { fontFamily: fonts.medium },
              headerShadowVisible: false,
              contentStyle: { backgroundColor: c.bg },
              headerBackTitle: 'Back',
            }}>
            <Stack.Protected guard={authed}>
              <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
              <Stack.Screen name="group/new" options={{ title: 'New group', presentation: 'modal' }} />
              <Stack.Screen name="group/[id]/index" options={{ title: 'Group' }} />
              <Stack.Screen name="group/[id]/add-receipt" options={{ title: 'Add receipt' }} />
              <Stack.Screen name="group/[id]/add-expense" options={{ title: 'Add expense' }} />
              <Stack.Screen name="notifications" options={{ title: 'Notifications' }} />
              <Stack.Screen name="person/[id]" options={{ title: 'Account' }} />
              <Stack.Screen name="receipt/[id]/index" options={{ title: 'Receipt' }} />
              <Stack.Screen name="receipt/[id]/edit" options={{ title: 'Edit items' }} />
              <Stack.Screen name="finance/transactions" options={{ title: 'Transactions' }} />
              <Stack.Screen name="finance/new-transaction" options={{ title: 'New transaction', presentation: 'modal' }} />
              <Stack.Screen name="finance/edit-transaction" options={{ title: 'Edit transaction' }} />
              <Stack.Screen name="finance/recurring" options={{ title: 'Subscriptions & recurring' }} />
              <Stack.Screen name="finance/import" options={{ title: 'Import statement' }} />
              <Stack.Screen name="finance/budgets" options={{ title: 'Budgets' }} />
              <Stack.Screen name="finance/goals" options={{ title: 'Savings goals' }} />
              <Stack.Screen name="finance/investments" options={{ title: 'Investments' }} />
              <Stack.Screen name="finance/investment" options={{ title: 'Investment', presentation: 'modal' }} />
              <Stack.Screen name="finance/asset" options={{ title: 'Asset' }} />
              <Stack.Screen name="finance/calculators" options={{ title: 'Calculators' }} />
              <Stack.Screen name="finance/net-worth" options={{ title: 'Net worth' }} />
              <Stack.Screen name="finance/account" options={{ title: 'Account', presentation: 'modal' }} />
              <Stack.Screen name="finance/export" options={{ title: 'Export' }} />
            </Stack.Protected>
            <Stack.Protected guard={!authed}>
              <Stack.Screen name="(auth)" options={{ headerShown: false }} />
            </Stack.Protected>
            <Stack.Screen name="invite/[token]" options={{ title: 'Invitation' }} />
          </Stack>
        </View>
      </View>
    </ThemeProvider>
  );
}

export default function RootLayout() {
  const [queryClient] = useState(makeQueryClient);
  const [fontsLoaded, fontError] = useFonts(fontAssets);
  useEffect(() => {
    // The app stays usable in the system font; the warning is for whoever debugs the missing typefaces.
    if (fontError) console.warn('Brand fonts failed to load, falling back to system fonts', fontError);
  }, [fontError]);
  const [themeReady, setThemeReady] = useState(false);
  useEffect(() => {
    // Restore the saved theme before the first frame so the app does not flash the other one.
    void loadThemePreference().finally(() => setThemeReady(true));
  }, []);
  if (!themeReady || (!fontsLoaded && !fontError)) return null;
  return (
    <SafeAreaProvider>
      <QueryClientProvider client={queryClient}>
        <AuthProvider>
          <RootNavigator />
        </AuthProvider>
      </QueryClientProvider>
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  shell: { flex: 1, flexDirection: 'row' },
  fill: { flex: 1 },
});
