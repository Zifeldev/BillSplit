import { useQuery } from '@tanstack/react-query';

import { BalanceApi, FinanceApi, FriendApi, GroupApi, NotificationApi, ReceiptApi } from './services';
import type { FriendLedger, MarketRange, Receipt, StatementImport } from './types';

export const qk = {
  me: ['me'] as const,
  groups: ['groups'] as const,
  group: (id: string) => ['group', id] as const,
  groupBalances: (id: string) => ['groupBalances', id] as const,
  receipts: (groupId: string) => ['receipts', groupId] as const,
  receipt: (id: string) => ['receipt', id] as const,
  split: (id: string) => ['split', id] as const,
  myBalances: ['myBalances'] as const,
  friendLedger: (userId: string) => ['friendLedger', userId] as const,
  friendLedgerAll: ['friendLedger'] as const,
  friends: ['friends'] as const,
  friendRequests: ['friendRequests'] as const,
  invite: (token: string) => ['invite', token] as const,
  currencies: ['currencies'] as const,
  categories: ['categories'] as const,
  transactions: (filter: object) => ['transactions', filter] as const,
  transactionsAll: ['transactions'] as const,
  budgets: ['budgets'] as const,
  goals: ['goals'] as const,
  dashboard: (year: number, month: number) => ['dashboard', year, month] as const,
  dashboardAll: ['dashboard'] as const,
  statementImport: (id: string) => ['statementImport', id] as const,
  statementImports: ['statementImports'] as const,
  groupInvites: (groupId: string) => ['groupInvites', groupId] as const,
  recurring: ['recurring'] as const,
  notifications: ['notifications'] as const,
  transaction: (id: string) => ['transaction', id] as const,
  portfolio: ['portfolio'] as const,
  marketOverview: ['marketOverview'] as const,
  marketChart: (symbol: string, range: string) => ['marketChart', symbol, range] as const,
  symbolSearch: (q: string) => ['symbolSearch', q] as const,
  netWorth: ['netWorth'] as const,
  forecast: ['forecast'] as const,
};

export const useGroups = () => useQuery({ queryKey: qk.groups, queryFn: GroupApi.listMine });

export const useGroup = (id: string) =>
  useQuery({ queryKey: qk.group(id), queryFn: () => GroupApi.get({ groupId: id }), enabled: !!id });

export const useGroupBalances = (id: string) =>
  useQuery({
    queryKey: qk.groupBalances(id),
    queryFn: () => BalanceApi.group({ groupId: id }),
    enabled: !!id,
  });

export const useReceipts = (groupId: string) =>
  useQuery({
    queryKey: qk.receipts(groupId),
    queryFn: () => ReceiptApi.list({ groupId, limit: 50, offset: 0 }),
    enabled: !!groupId,
  });

function isProcessing(r: Receipt | undefined): boolean {
  return !r || r.status === 'RECEIPT_STATUS_PENDING' || r.status === 'RECEIPT_STATUS_UNSPECIFIED';
}

/** Polls every 2s while OCR is still running. */
export const useReceipt = (id: string) =>
  useQuery({
    queryKey: qk.receipt(id),
    queryFn: () => ReceiptApi.get({ receiptId: id }),
    enabled: !!id,
    refetchInterval: (q) => (q.state.error ? false : isProcessing(q.state.data) ? 2000 : false),
  });

export const useSplit = (id: string, enabled: boolean) =>
  useQuery({
    queryKey: qk.split(id),
    queryFn: () => ReceiptApi.split({ receiptId: id }),
    enabled: !!id && enabled,
  });

export const useMyBalances = () => useQuery({ queryKey: qk.myBalances, queryFn: BalanceApi.mine });

/** Shared account with one person across all common groups. Missing repeated/zero fields are filled in. */
export const useFriendLedger = (userId: string) =>
  useQuery({
    queryKey: qk.friendLedger(userId),
    queryFn: async (): Promise<FriendLedger> => {
      const l = await BalanceApi.friendLedger({ userId });
      return {
        userId: l.userId || userId,
        name: l.name ?? '',
        payment: l.payment ?? null,
        currency: l.currency || 'USD',
        netCents: l.netCents ?? 0,
        groups: (l.groups ?? []).map((g) => ({
          groupId: g.groupId ?? '',
          groupName: g.groupName ?? '',
          currency: g.currency || l.currency || 'USD',
          netCents: g.netCents ?? 0,
          netHomeCents: g.netHomeCents ?? 0,
          entries: (g.entries ?? []).map((e) => ({
            kind: e.kind ?? 'LEDGER_ENTRY_KIND_UNSPECIFIED',
            receiptId: e.receiptId ?? '',
            title: e.title ?? '',
            amountCents: e.amountCents ?? 0,
            occurredAt: e.occurredAt ?? null,
          })),
        })),
      };
    },
    enabled: !!userId,
  });

export const useFriends = () => useQuery({ queryKey: qk.friends, queryFn: FriendApi.list });

export const useFriendRequests = () =>
  useQuery({ queryKey: qk.friendRequests, queryFn: FriendApi.requests });

export const useCurrencies = (enabled = true) =>
  useQuery({ queryKey: qk.currencies, queryFn: FinanceApi.currencies, staleTime: 60 * 60 * 1000, enabled });

export const useCategories = () =>
  useQuery({ queryKey: qk.categories, queryFn: FinanceApi.categories, staleTime: 10 * 60 * 1000 });

export const useBudgets = () => useQuery({ queryKey: qk.budgets, queryFn: FinanceApi.listBudgets });

export const useGoals = () => useQuery({ queryKey: qk.goals, queryFn: FinanceApi.listGoals });

export const useDashboard = (year: number, month: number) =>
  useQuery({
    queryKey: qk.dashboard(year, month),
    queryFn: () => FinanceApi.dashboard({ year, month }),
  });

export const usePortfolio = () => useQuery({ queryKey: qk.portfolio, queryFn: FinanceApi.portfolio });

export const useNetWorth = () => useQuery({ queryKey: qk.netWorth, queryFn: FinanceApi.netWorth });

export const useForecast = () => useQuery({ queryKey: qk.forecast, queryFn: FinanceApi.forecast });

export const useMarketOverview = () =>
  useQuery({ queryKey: qk.marketOverview, queryFn: FinanceApi.marketOverview, staleTime: 5 * 60 * 1000 });

export const useMarketChart = (symbol: string, range: MarketRange) =>
  useQuery({
    queryKey: qk.marketChart(symbol, range),
    queryFn: () => FinanceApi.marketChart({ symbol, range }),
    enabled: !!symbol,
    staleTime: 5 * 60 * 1000,
    placeholderData: (prev) => prev,
  });

export const useSymbolSearch = (q: string) =>
  useQuery({
    queryKey: qk.symbolSearch(q),
    queryFn: () => FinanceApi.searchSymbols(q),
    enabled: q.trim().length >= 2,
    staleTime: 60 * 60 * 1000,
  });

export function importRunning(s: StatementImport | undefined): boolean {
  return !s || s.status === 'IMPORT_STATUS_PENDING' || s.status === 'IMPORT_STATUS_UNSPECIFIED';
}

export const useStatementImport = (id: string | null) =>
  useQuery({
    queryKey: qk.statementImport(id ?? ''),
    queryFn: () => FinanceApi.getStatementImport({ importId: id ?? '' }),
    enabled: !!id,
    refetchInterval: (q) => (q.state.error ? false : importRunning(q.state.data) ? 2000 : false),
  });

export const useStatementImports = () =>
  useQuery({
    queryKey: qk.statementImports,
    queryFn: () => FinanceApi.listStatementImports({ limit: 20 }),
    refetchInterval: (q) =>
      q.state.error ? false : (q.state.data?.imports ?? []).some((i) => importRunning(i)) ? 3000 : false,
  });

export const useGroupInvites = (groupId: string) =>
  useQuery({
    queryKey: qk.groupInvites(groupId),
    queryFn: () => GroupApi.listInvites({ groupId }),
    enabled: !!groupId,
  });

export const useRecurring = () => useQuery({ queryKey: qk.recurring, queryFn: FinanceApi.listRecurring });

/** Inbox; polls every 30s while `polling` (the screen showing it is focused). */
export const useNotifications = (polling: boolean, enabled = true) =>
  useQuery({
    queryKey: qk.notifications,
    queryFn: () => NotificationApi.list({ limit: 50, unreadOnly: false }),
    enabled,
    refetchInterval: polling ? 30_000 : false,
    refetchIntervalInBackground: false,
  });
