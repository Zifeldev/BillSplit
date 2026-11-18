import { rpc } from './client';
import type * as T from './types';

const empty: T.Empty = {};

export const AuthApi = {
  register: (req: T.RegisterRequest) =>
    rpc<T.RegisterRequest, T.AuthResponse>('AuthService', 'Register', req),
  login: (req: T.LoginRequest) => rpc<T.LoginRequest, T.AuthResponse>('AuthService', 'Login', req),
  me: () => rpc<T.MeRequest, T.User>('AuthService', 'Me', empty),
  updateProfile: (req: T.UpdateProfileRequest) =>
    rpc<T.UpdateProfileRequest, T.User>('AuthService', 'UpdateProfile', req),
  logout: (req: T.LogoutRequest) => rpc<T.LogoutRequest, T.LogoutResponse>('AuthService', 'Logout', req),
};

export const FriendApi = {
  list: () => rpc<T.Empty, T.ListFriendsResponse>('FriendService', 'ListFriends', empty),
  requests: () =>
    rpc<T.Empty, T.ListFriendRequestsResponse>('FriendService', 'ListFriendRequests', empty),
  send: (req: T.SendFriendRequestRequest) =>
    rpc<T.SendFriendRequestRequest, T.Friend>('FriendService', 'SendFriendRequest', req),
  respond: (req: T.RespondFriendRequestRequest) =>
    rpc<T.RespondFriendRequestRequest, T.Friend>('FriendService', 'RespondFriendRequest', req),
  remove: (req: T.RemoveFriendRequest) =>
    rpc<T.RemoveFriendRequest, T.RemoveFriendResponse>('FriendService', 'RemoveFriend', req),
  createInvite: () =>
    rpc<T.CreateFriendInviteRequest, T.InviteLink>('FriendService', 'CreateFriendInvite', empty),
  acceptInvite: (req: T.AcceptFriendInviteRequest) =>
    rpc<T.AcceptFriendInviteRequest, T.Friend>('FriendService', 'AcceptFriendInvite', req),
};

export const GroupApi = {
  create: (req: T.CreateGroupRequest) =>
    rpc<T.CreateGroupRequest, T.Group>('GroupService', 'CreateGroup', req),
  get: (req: T.GetGroupRequest) => rpc<T.GetGroupRequest, T.Group>('GroupService', 'GetGroup', req),
  listMine: () => rpc<T.Empty, T.ListMyGroupsResponse>('GroupService', 'ListMyGroups', empty),
  update: (req: T.UpdateGroupRequest) =>
    rpc<T.UpdateGroupRequest, T.Group>('GroupService', 'UpdateGroup', req),
  addMember: (req: T.AddMemberRequest) =>
    rpc<T.AddMemberRequest, T.Group>('GroupService', 'AddMember', req),
  removeMember: (req: T.RemoveMemberRequest) =>
    rpc<T.RemoveMemberRequest, T.Group>('GroupService', 'RemoveMember', req),
  createInvite: (req: T.CreateGroupInviteRequest) =>
    rpc<T.CreateGroupInviteRequest, T.InviteLink>('GroupService', 'CreateGroupInvite', req),
  listInvites: (req: T.ListGroupInvitesRequest) =>
    rpc<T.ListGroupInvitesRequest, T.ListGroupInvitesResponse>('GroupService', 'ListGroupInvites', req),
  revokeInvite: (req: T.RevokeGroupInviteRequest) =>
    rpc<T.RevokeGroupInviteRequest, T.RevokeGroupInviteResponse>('GroupService', 'RevokeGroupInvite', req),
  /** Public: works without login (alreadyMember is false for anonymous callers). */
  getInvite: (req: T.GetInviteRequest) =>
    rpc<T.GetInviteRequest, T.InvitePreview>('GroupService', 'GetInvite', req),
  join: (req: T.JoinGroupRequest) => rpc<T.JoinGroupRequest, T.Group>('GroupService', 'JoinGroup', req),
};

export const ReceiptApi = {
  upload: (req: T.UploadReceiptRequest) =>
    rpc<T.UploadReceiptRequest, T.Receipt>('ReceiptService', 'UploadReceipt', req),
  create: (req: T.CreateReceiptRequest) =>
    rpc<T.CreateReceiptRequest, T.Receipt>('ReceiptService', 'CreateReceipt', req),
  createExpense: (req: T.CreateExpenseRequest) =>
    rpc<T.CreateExpenseRequest, T.Split>('ReceiptService', 'CreateExpense', req),
  get: (req: T.GetReceiptRequest) =>
    rpc<T.GetReceiptRequest, T.Receipt>('ReceiptService', 'GetReceipt', req),
  list: (req: T.ListReceiptsRequest) =>
    rpc<T.ListReceiptsRequest, T.ListReceiptsResponse>('ReceiptService', 'ListReceipts', req),
  updateItems: (req: T.UpdateItemsRequest) =>
    rpc<T.UpdateItemsRequest, T.Receipt>('ReceiptService', 'UpdateItems', req),
  claim: (req: T.ClaimItemsRequest) =>
    rpc<T.ClaimItemsRequest, T.Receipt>('ReceiptService', 'ClaimItems', req),
  split: (req: T.GetSplitRequest) => rpc<T.GetSplitRequest, T.Split>('ReceiptService', 'GetSplit', req),
  finalize: (req: T.FinalizeReceiptRequest) =>
    rpc<T.FinalizeReceiptRequest, T.Split>('ReceiptService', 'FinalizeReceipt', req),
  remove: (req: T.DeleteReceiptRequest) =>
    rpc<T.DeleteReceiptRequest, T.DeleteReceiptResponse>('ReceiptService', 'DeleteReceipt', req),
};

export const BalanceApi = {
  group: (req: T.GetBalancesRequest) =>
    rpc<T.GetBalancesRequest, T.GetBalancesResponse>('BalanceService', 'GetBalances', req),
  settleUp: (req: T.SettleUpRequest) =>
    rpc<T.SettleUpRequest, T.Settlement>('BalanceService', 'SettleUp', req),
  mine: () => rpc<T.Empty, T.GetMyBalancesResponse>('BalanceService', 'GetMyBalances', empty),
  /** One account with a person across all common groups (debts in one group offset credits in another). */
  friendLedger: (req: T.GetFriendLedgerRequest) =>
    rpc<T.GetFriendLedgerRequest, T.FriendLedger>('BalanceService', 'GetFriendLedger', req),
  /** Records a settlement in every common group so each pairwise balance becomes zero. */
  settleWithFriend: (req: T.SettleWithFriendRequest) =>
    rpc<T.SettleWithFriendRequest, T.SettleWithFriendResponse>('BalanceService', 'SettleWithFriend', req),
};

export const FinanceApi = {
  /** Public: works without login. */
  currencies: () => rpc<T.Empty, T.ListCurrenciesResponse>('FinanceService', 'ListCurrencies', empty),
  categories: () => rpc<T.Empty, T.ListCategoriesResponse>('FinanceService', 'ListCategories', empty),
  createTransaction: (req: T.CreateTransactionRequest) =>
    rpc<T.CreateTransactionRequest, T.Transaction>('FinanceService', 'CreateTransaction', req),
  listTransactions: (req: T.ListTransactionsRequest) =>
    rpc<T.ListTransactionsRequest, T.ListTransactionsResponse>('FinanceService', 'ListTransactions', req),
  updateTransactionCategory: (req: T.UpdateTransactionCategoryRequest) =>
    rpc<T.UpdateTransactionCategoryRequest, T.Transaction>(
      'FinanceService',
      'UpdateTransactionCategory',
      req,
    ),
  updateTransaction: (req: T.UpdateTransactionRequest) =>
    rpc<T.UpdateTransactionRequest, T.Transaction>('FinanceService', 'UpdateTransaction', req),
  deleteTransaction: (req: T.DeleteTransactionRequest) =>
    rpc<T.DeleteTransactionRequest, T.Empty>('FinanceService', 'DeleteTransaction', req),
  importStatement: (req: T.ImportStatementRequest) =>
    rpc<T.ImportStatementRequest, T.StatementImport>('FinanceService', 'ImportStatement', req),
  getStatementImport: (req: T.GetStatementImportRequest) =>
    rpc<T.GetStatementImportRequest, T.StatementImport>('FinanceService', 'GetStatementImport', req),
  listStatementImports: (req: T.ListStatementImportsRequest) =>
    rpc<T.ListStatementImportsRequest, T.ListStatementImportsResponse>(
      'FinanceService',
      'ListStatementImports',
      req,
    ),
  setBudget: (req: T.SetBudgetRequest) =>
    rpc<T.SetBudgetRequest, T.Budget>('FinanceService', 'SetBudget', req),
  listBudgets: () => rpc<T.Empty, T.ListBudgetsResponse>('FinanceService', 'ListBudgets', empty),
  deleteBudget: (req: T.DeleteBudgetRequest) =>
    rpc<T.DeleteBudgetRequest, T.DeleteBudgetResponse>('FinanceService', 'DeleteBudget', req),
  setGoal: (req: T.SetSavingsGoalRequest) =>
    rpc<T.SetSavingsGoalRequest, T.SavingsGoal>('FinanceService', 'SetSavingsGoal', req),
  listGoals: () => rpc<T.Empty, T.ListSavingsGoalsResponse>('FinanceService', 'ListSavingsGoals', empty),
  deleteGoal: (req: T.DeleteSavingsGoalRequest) =>
    rpc<T.DeleteSavingsGoalRequest, T.DeleteSavingsGoalResponse>('FinanceService', 'DeleteSavingsGoal', req),
  listRecurring: () =>
    rpc<T.ListRecurringRequest, T.ListRecurringResponse>('FinanceService', 'ListRecurring', empty),
  dashboard: (req: T.GetDashboardRequest) =>
    rpc<T.GetDashboardRequest, T.Dashboard>('FinanceService', 'GetDashboard', req),
  portfolio: () => rpc<T.Empty, T.Portfolio>('FinanceService', 'ListInvestments', empty),
  netWorth: () => rpc<T.Empty, T.NetWorth>('FinanceService', 'GetNetWorth', empty),
  setAccount: (req: T.SetAccountRequest) => rpc<T.SetAccountRequest, T.Account>('FinanceService', 'SetAccount', req),
  deleteAccount: (req: { id: string }) => rpc<{ id: string }, T.Empty>('FinanceService', 'DeleteAccount', req),
  forecast: () => rpc<T.Empty, T.MonthForecast>('FinanceService', 'GetMonthForecast', empty),
  exportReport: (req: T.ExportReportRequest) =>
    rpc<T.ExportReportRequest, T.ExportFile>('FinanceService', 'ExportReport', req),
  setInvestment: (req: T.SetInvestmentRequest) =>
    rpc<T.SetInvestmentRequest, T.Investment>('FinanceService', 'SetInvestment', req),
  deleteInvestment: (req: T.DeleteInvestmentRequest) =>
    rpc<T.DeleteInvestmentRequest, T.Empty>('FinanceService', 'DeleteInvestment', req),
  marketChart: (req: T.GetMarketChartRequest) =>
    rpc<T.GetMarketChartRequest, T.MarketChart>('FinanceService', 'GetMarketChart', req),
  searchSymbols: (query: string) =>
    rpc<{ query: string }, T.SearchSymbolsResponse>('FinanceService', 'SearchSymbols', { query }),
  marketOverview: () => rpc<T.Empty, T.MarketOverview>('FinanceService', 'GetMarketOverview', empty),
};

export const NotificationApi = {
  registerDevice: (req: T.RegisterDeviceRequest) =>
    rpc<T.RegisterDeviceRequest, T.RegisterDeviceResponse>('NotificationService', 'RegisterDevice', req),
  unregisterDevice: (req: T.UnregisterDeviceRequest) =>
    rpc<T.UnregisterDeviceRequest, T.UnregisterDeviceResponse>('NotificationService', 'UnregisterDevice', req),
  list: (req: T.ListNotificationsRequest) =>
    rpc<T.ListNotificationsRequest, T.ListNotificationsResponse>('NotificationService', 'ListNotifications', req),
  markRead: (req: T.MarkNotificationsReadRequest) =>
    rpc<T.MarkNotificationsReadRequest, T.MarkNotificationsReadResponse>(
      'NotificationService',
      'MarkNotificationsRead',
      req,
    ),
};
