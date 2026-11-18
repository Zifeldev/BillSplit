import { readAsStringAsync } from 'expo-file-system/legacy';

/** Reads a local file URI (from document/image picker) as a base64 string. */
export async function readFileAsBase64(uri: string): Promise<string> {
  return readAsStringAsync(uri, { encoding: 'base64' });
}
