import { Redirect } from 'expo-router';
import { Platform } from 'react-native';

import { Landing } from '@/components/landing/Landing';

/** Signed-out entry point: the public landing page on the web, straight to login in the app. */
export default function AuthIndex() {
  if (Platform.OS === 'web') return <Landing />;
  return <Redirect href="/login" />;
}
