import { Redirect } from 'expo-router';

/** Placeholder route for the centre "+" tab; the tab button opens a sheet instead. */
export default function AddPlaceholder() {
  return <Redirect href="/" />;
}
