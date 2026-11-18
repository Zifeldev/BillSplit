import { cacheDirectory, writeAsStringAsync } from 'expo-file-system/legacy';
import { Share } from 'react-native';

/** Native: writes the file to the cache and opens the share sheet (Save to Files, Mail, …). */
export async function saveFile(filename: string, _contentType: string, base64: string): Promise<void> {
  const uri = `${cacheDirectory}${filename}`;
  await writeAsStringAsync(uri, base64, { encoding: 'base64' });
  await Share.share({ url: uri, title: filename });
}
