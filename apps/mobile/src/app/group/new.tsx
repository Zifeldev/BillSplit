import { Icon } from '@/components/Icon';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { View } from 'react-native';

import { errorMessage } from '@/api/client';
import { qk, useFriends } from '@/api/queries';
import { GroupApi } from '@/api/services';
import { useAuth } from '@/auth/AuthContext';
import { CurrencyField } from '@/components/CurrencyField';
import { Avatar, Banner, Button, Card, Field, HeroCard, ListRow, Row, Screen, SectionHeader, SkeletonList, Txt } from '@/components/ui';
import { groupCover } from '@/lib/colors';
import { useTheme } from '@/theme';

export default function NewGroupScreen() {
  const router = useRouter();
  const qc = useQueryClient();
  const { c } = useTheme();
  const { user } = useAuth();
  const friends = useFriends();
  const [name, setName] = useState('');
  const [currency, setCurrency] = useState(user?.currency || 'USD');
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [error, setError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => GroupApi.create({ name: name.trim(), currency, friendIds: [...selected] }),
    onSuccess: (g) => {
      void qc.invalidateQueries({ queryKey: qk.groups });
      router.replace(`/group/${g.id}`);
    },
    onError: (e) => setError(errorMessage(e)),
  });

  const toggle = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  // Preview cover: keyword icon from the name, stable colors.
  const cover = groupCover('new', name);
  const accepted = (friends.data?.friends ?? []).filter((f) => f.status === 'FRIEND_STATUS_ACCEPTED' || f.status === 'FRIEND_STATUS_UNSPECIFIED');

  return (
    <Screen>
      {error ? <Banner kind="error" message={error} /> : null}
      <HeroCard color={cover.color}>
        <Row gap={14}>
          <View style={{ width: 60, height: 60, borderRadius: 20, backgroundColor: 'rgba(255,255,255,0.22)', alignItems: 'center', justifyContent: 'center' }}>
            <Icon name={cover.icon} size={30} color="#FFFFFF" weight="duotone" />
          </View>
          <View style={{ flex: 1 }}>
            <Txt variant="h2" color="#FFFFFF" numberOfLines={1}>
              {name.trim() || 'New group'}
            </Txt>
            <Txt variant="small" color="rgba(255,255,255,0.88)">
              {selected.size + 1} {selected.size === 0 ? 'member' : 'members'} · {currency}
            </Txt>
          </View>
        </Row>
      </HeroCard>
      <Card>
        <Field label="Group name" value={name} onChangeText={setName} placeholder="Trip to Lisbon" />
        <CurrencyField label="Group currency" value={currency} onChange={setCurrency} />
        <Txt variant="small">Balances are kept in this currency. Receipts in other currencies are converted.</Txt>
      </Card>

      <SectionHeader title={`Add friends (${selected.size})`} />
      <Card>
        {friends.isLoading ? (
          <SkeletonList rows={2} card={false} />
        ) : accepted.length === 0 ? (
          <Txt variant="muted">No friends yet — you can invite people with a link after creating the group.</Txt>
        ) : (
          accepted.map((f, i) => (
            <ListRow
              divider={i > 0}
              key={f.userId}
              left={<Avatar name={f.name} id={f.userId} />}
              title={f.name || f.email}
              subtitle={f.email}
              onPress={() => toggle(f.userId)}
              right={
                <Icon
                  name={selected.has(f.userId) ? 'checkmark-circle' : 'ellipse-outline'}
                  size={24}
                  color={selected.has(f.userId) ? c.primary : c.textMuted}
                />
              }
            />
          ))
        )}
      </Card>

      <Button
        title="Create group"
        icon="checkmark"
        disabled={!name.trim()}
        loading={create.isPending}
        onPress={() => {
          setError(null);
          create.mutate();
        }}
      />
    </Screen>
  );
}
