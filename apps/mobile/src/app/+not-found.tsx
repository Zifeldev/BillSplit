import { Stack, useRouter } from 'expo-router';
import React from 'react';

import { Button, EmptyView, Screen } from '@/components/ui';

export default function NotFound() {
  const router = useRouter();
  return (
    <Screen>
      <Stack.Screen options={{ title: 'Not found' }} />
      <EmptyView
        icon="help-circle-outline"
        title="Page not found"
        message="This link doesn't lead anywhere."
        action={<Button title="Go home" onPress={() => router.replace('/')} />}
      />
    </Screen>
  );
}
