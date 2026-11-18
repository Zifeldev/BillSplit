/**
 * Hand-written TypeScript mirrors of the proto messages in api/proto/billsplit/v1.
 *
 * JSON conventions (grpc-gateway / protojson):
 *  - lowerCamelCase field names
 *  - int64 fields (all `*Cents`) arrive as strings; the client converts every
 *    `...Cents` key to `number` before handing data to the app, so these types
 *    describe the *normalized* shape. Requests may send int64 as JSON numbers.
 *  - ALL money (`*Cents`) is in MINOR UNITS of its currency (Currency.minorUnits:
 *    2 for USD, 0 for JPY/KRW) — never assume 100. See lib/format.ts.
 *  - `optional` proto fields are `?` here; omitted means "not set".
 *  - enums are their string names
 *  - Timestamps are RFC3339 strings
 *  - bytes are base64 strings
 */

export type Timestamp = string;

// ---------- auth.proto ----------
export interface PaymentHandles {
  venmo: string; // Venmo username without @
  paypal: string; // PayPal.me username
  zelle: string; // email or phone registered in Zelle
}
export interface User {
  id: string;
  email: string;
  name: string;
  currency: string;
  createdAt: Timestamp | null;
  payment: PaymentHandles | null;
}

export interface RegisterRequest {
  email: string;
  password: string;
  name: string;
  currency: string;
}
export interface LoginRequest {
  email: string;
  password: string;
}
export interface RefreshRequest {
  refreshToken: string;
}
export type MeRequest = Record<string, never>;
export interface UpdateProfileRequest {
  name: string; // "" — keep
  currency: string; // "" — keep
  /** Set only the handles you want to change; "" clears a handle. */
  venmo?: string;
  paypal?: string;
  zelle?: string;
}
export interface LogoutRequest {
  refreshToken: string;
}
export type LogoutResponse = Record<string, never>;
export interface AuthResponse {
  accessToken: string;
  refreshToken: string;
  accessExpiresAt: Timestamp | null;
  user: User | null;
}

// ---------- social.proto ----------
export type FriendStatus =
  | 'FRIEND_STATUS_UNSPECIFIED'
  | 'FRIEND_STATUS_PENDING_INCOMING'
  | 'FRIEND_STATUS_PENDING_OUTGOING'
  | 'FRIEND_STATUS_ACCEPTED';

export interface Friend {
  userId: string;
  name: string;
  email: string;
  status: FriendStatus;
  since: Timestamp | null;
  payment: PaymentHandles | null; // only for accepted friends
}
export interface ListFriendsResponse {
  friends: Friend[];
}
export interface ListFriendRequestsResponse {
  incoming: Friend[];
  outgoing: Friend[];
}
export interface SendFriendRequestRequest {
  email: string;
}
export interface RespondFriendRequestRequest {
  userId: string;
  accept: boolean;
}
export interface RemoveFriendRequest {
  userId: string;
}
export type RemoveFriendResponse = Record<string, never>;
export type CreateFriendInviteRequest = Record<string, never>;

export type InviteKind = 'INVITE_KIND_UNSPECIFIED' | 'INVITE_KIND_FRIEND' | 'INVITE_KIND_GROUP';

export interface InviteLink {
  token: string;
  url: string;
  appUrl: string;
  kind: InviteKind;
  expiresAt: Timestamp | null;
  maxUses: number;
  uses: number;
}
export interface AcceptFriendInviteRequest {
  token: string;
}

export interface GroupMember {
  userId: string;
  name: string;
  email: string;
  role: string; // owner | member
  payment: PaymentHandles | null;
}
export interface Group {
  id: string;
  name: string;
  ownerId: string;
  currency: string;
  members: GroupMember[];
  createdAt: Timestamp | null;
}
export interface CreateGroupRequest {
  name: string;
  currency: string;
  friendIds: string[];
}
export interface GetGroupRequest {
  groupId: string;
}
export interface ListMyGroupsResponse {
  groups: Group[];
}
export interface UpdateGroupRequest {
  groupId: string;
  name: string;
  currency: string; // allowed only while the group has no finalized receipts
}
export interface AddMemberRequest {
  groupId: string;
  userId: string;
}
export interface RemoveMemberRequest {
  groupId: string;
  userId: string;
}
export interface CreateGroupInviteRequest {
  groupId: string;
  ttlHours: number; // default 72, max 720
  maxUses: number; // 0 — unlimited
}
export interface ListGroupInvitesRequest {
  groupId: string;
}
/** Active (not expired/revoked) only. */
export interface ListGroupInvitesResponse {
  invites: InviteLink[];
}
export interface RevokeGroupInviteRequest {
  token: string;
}
export type RevokeGroupInviteResponse = Record<string, never>;
export interface GetInviteRequest {
  token: string;
}
export interface InvitePreview {
  kind: InviteKind;
  inviterName: string;
  groupId: string;
  groupName: string;
  membersCount: number;
  expired: boolean;
  alreadyMember: boolean;
}
export interface JoinGroupRequest {
  token: string;
}

// ---------- split.proto ----------
export type ReceiptStatus =
  | 'RECEIPT_STATUS_UNSPECIFIED'
  | 'RECEIPT_STATUS_PENDING'
  | 'RECEIPT_STATUS_PARSED'
  | 'RECEIPT_STATUS_FAILED'
  | 'RECEIPT_STATUS_FINALIZED';

export interface ItemClaim {
  userId: string;
  shares: number;
}
export interface ReceiptItem {
  id: string;
  /** Readable name ("Great Value Half & Half"); falls back to rawName on the server. */
  name: string;
  quantity: number;
  priceCents: number;
  claims: ItemClaim[];
  /** As printed on the receipt ("GV HH 320Z"); empty for manual items. */
  rawName?: string;
  /** UPC/EAN printed on the receipt, if any. */
  code?: string;
}
export interface Receipt {
  id: string;
  groupId: string;
  payerId: string;
  store: string;
  status: ReceiptStatus;
  items: ReceiptItem[];
  subtotalCents: number;
  taxCents: number;
  tipCents: number;
  totalCents: number;
  currency: string;
  parseError: string;
  ocrProvider: string;
  purchasedAt: Timestamp | null;
  createdAt: Timestamp | null;
  kind: ReceiptKind;
  payerName: string;
}
export type ReceiptKind = 'RECEIPT_KIND_UNSPECIFIED' | 'RECEIPT_KIND_RECEIPT' | 'RECEIPT_KIND_EXPENSE';

/** `oneof source { bytes image; string raw_text; }` — exactly one is set. */
export type UploadReceiptRequest = {
  groupId: string;
  currency: string; // "" — group currency
  tipCents: number;
  // `image`: base64 of a jpeg/png/webp photo or a PDF receipt, up to 8 MB.
} & ({ image: string; rawText?: never } | { rawText: string; image?: never });

export interface CreateReceiptRequest {
  groupId: string;
  store: string;
  currency: string; // "" — group currency
  items: ItemInput[];
  taxCents: number;
  tipCents: number;
  purchasedAt?: Timestamp;
}

export type SplitType =
  | 'SPLIT_TYPE_UNSPECIFIED'
  | 'SPLIT_TYPE_EQUAL' // value ignored
  | 'SPLIT_TYPE_PERCENT' // value = basis points (2500 = 25%), must sum to 10000
  | 'SPLIT_TYPE_EXACT' // value = minor units, must sum to amount
  | 'SPLIT_TYPE_SHARES'; // value = weight (1, 2, ...)

export interface ExpenseParticipant {
  userId: string;
  value: number; // int64
}
export interface CreateExpenseRequest {
  groupId: string;
  description: string;
  amountCents: number;
  currency: string; // "" — group currency
  paidBy: string; // "" — caller
  splitType: SplitType;
  participants: ExpenseParticipant[]; // empty with EQUAL — all members
  occurredAt?: Timestamp;
}

export interface GetReceiptRequest {
  receiptId: string;
}
export interface ListReceiptsRequest {
  groupId: string;
  limit?: number;
  offset?: number;
}
export interface ListReceiptsResponse {
  receipts: Receipt[];
}
export interface ItemInput {
  name: string;
  quantity: number;
  priceCents: number;
}
export interface UpdateItemsRequest {
  receiptId: string;
  items: ItemInput[];
  store: string;
  taxCents: number;
  tipCents: number;
  currency: string;
}
export interface ClaimInput {
  itemId: string;
  shares: number;
}
export interface ClaimItemsRequest {
  receiptId: string;
  claims: ClaimInput[];
}
export interface MemberShare {
  userId: string;
  itemsCents: number;
  taxCents: number;
  tipCents: number;
  totalCents: number; // receipt currency
  totalGroupCents: number; // converted to group currency
  name: string;
}
export interface Split {
  receiptId: string;
  payerId: string;
  shares: MemberShare[];
  unclaimedCents: number;
  unclaimedItemIds: string[];
  currency: string;
  groupCurrency: string;
  fxRate: number;
}
export type FinalizeReceiptRequest = GetReceiptRequest;
export type DeleteReceiptRequest = GetReceiptRequest;
export type DeleteReceiptResponse = Record<string, never>;
export type GetSplitRequest = GetReceiptRequest;
export interface GetBalancesRequest {
  groupId: string;
}
export interface Transfer {
  fromUserId: string;
  toUserId: string;
  amountCents: number;
  fromName: string;
  toName: string;
  toPayment: PaymentHandles | null; // for "pay with Venmo/PayPal/Zelle" buttons
}
export interface MemberBalance {
  userId: string;
  netCents: number; // >0 — is owed, <0 — owes
  name: string;
}
export interface GetBalancesResponse {
  currency: string;
  balances: MemberBalance[];
  suggestedTransfers: Transfer[];
}
/** Caller must be the debtor ("I paid") or the creditor ("I received"). */
export interface SettleUpRequest {
  groupId: string;
  toUserId: string;
  amountCents: number; // group currency
  fromUserId: string; // "" — caller
}
export interface Settlement {
  id: string;
  groupId: string;
  fromUserId: string;
  toUserId: string;
  amountCents: number;
  currency: string;
  createdAt: Timestamp | null;
}
export interface FriendBalance {
  userId: string;
  netCents: number; // home currency; >0 — friend owes you
  name: string;
  payment: PaymentHandles | null;
}
export interface GetMyBalancesResponse {
  currency: string;
  owedToMeCents: number;
  iOweCents: number;
  friends: FriendBalance[];
}

/** Kind of event in a friend ledger. */
export type LedgerEntryKind =
  | 'LEDGER_ENTRY_KIND_UNSPECIFIED'
  | 'LEDGER_ENTRY_KIND_RECEIPT' // itemized receipt
  | 'LEDGER_ENTRY_KIND_EXPENSE' // single-amount expense
  | 'LEDGER_ENTRY_KIND_SETTLEMENT'; // payment between the two of you
export interface GetFriendLedgerRequest {
  userId: string;
}
/** One event between you and the friend, from YOUR side (group currency). */
export interface LedgerEntry {
  kind: LedgerEntryKind;
  receiptId: string; // "" for settlements
  title: string; // store / expense description / "Payment"
  amountCents: number; // >0 — they owe you more, <0 — you owe them more
  occurredAt: Timestamp | null;
}
export interface LedgerGroup {
  groupId: string;
  groupName: string;
  currency: string; // group currency
  netCents: number; // group currency; >0 — friend owes you
  netHomeCents: number; // converted to your home currency
  entries: LedgerEntry[]; // newest first
}
/** One shared account with a person across every common group (cross-group netting). */
export interface FriendLedger {
  userId: string;
  name: string;
  payment: PaymentHandles | null;
  currency: string; // your home currency
  netCents: number; // home currency; >0 — friend owes you
  groups: LedgerGroup[]; // only groups with history between you two
}
export interface SettleWithFriendRequest {
  userId: string;
}
export interface SettleWithFriendResponse {
  settlements: Settlement[]; // one per group that had a non-zero balance
  netPaidCents: number; // home currency; >0 — friend paid you, <0 — you paid
  currency: string;
}

// ---------- finance.proto ----------
export type CategoryKind =
  | 'CATEGORY_KIND_UNSPECIFIED'
  | 'CATEGORY_KIND_NEEDS'
  | 'CATEGORY_KIND_WANTS'
  | 'CATEGORY_KIND_SAVINGS'
  | 'CATEGORY_KIND_INCOME';

export interface Category {
  id: string;
  name: string;
  kind: CategoryKind;
  icon: string;
}
export interface Currency {
  code: string;
  name: string;
  /** Digits after the decimal point (ISO 4217): 2 for USD, 0 for JPY/KRW. */
  minorUnits: number;
}
export interface ListCurrenciesResponse {
  currencies: Currency[];
}
export interface ListCategoriesResponse {
  categories: Category[];
}
export interface Transaction {
  id: string;
  amountCents: number;
  currency: string;
  description: string;
  merchant: string;
  categoryId: string;
  source: string; // manual | statement | receipt
  occurredAt: Timestamp | null;
}
export interface CreateTransactionRequest {
  amountCents: number;
  description: string;
  merchant: string;
  categoryId?: string;
  occurredAt?: Timestamp;
  currency?: string;
}
export interface ListTransactionsRequest {
  from?: Timestamp;
  to?: Timestamp;
  categoryId?: string;
  limit?: number;
  offset?: number;
}
export interface ListTransactionsResponse {
  transactions: Transaction[];
}
export interface UpdateTransactionCategoryRequest {
  transactionId: string;
  categoryId: string;
  rememberRule: boolean;
}
/** Only set fields are changed. */
export interface UpdateTransactionRequest {
  transactionId: string;
  amountCents?: number;
  currency?: string;
  description?: string;
  merchant?: string;
  categoryId?: string;
  occurredAt?: Timestamp; // omitted — keep
}
export interface DeleteTransactionRequest {
  transactionId: string;
}
export type StatementFormat =
  | 'STATEMENT_FORMAT_UNSPECIFIED'
  | 'STATEMENT_FORMAT_CSV'
  | 'STATEMENT_FORMAT_OFX'
  | 'STATEMENT_FORMAT_PDF';

export interface ImportStatementRequest {
  file: string; // base64
  format: StatementFormat;
  bank?: string;
  /** Default currency for rows without one (multi-currency statements carry it per row); empty — home currency. */
  currency?: string;
}
export type ImportStatus =
  | 'IMPORT_STATUS_UNSPECIFIED'
  | 'IMPORT_STATUS_PENDING'
  | 'IMPORT_STATUS_DONE'
  | 'IMPORT_STATUS_FAILED';
export interface StatementImport {
  id: string;
  status: ImportStatus;
  imported: number;
  duplicates: number;
  error: string;
  createdAt: Timestamp | null;
}
export interface GetStatementImportRequest {
  importId: string;
}
export interface ListStatementImportsRequest {
  limit: number;
}
export interface ListStatementImportsResponse {
  imports: StatementImport[];
}
export interface Budget {
  id: string;
  categoryId: string;
  limitCents: number;
}
export interface SetBudgetRequest {
  categoryId: string;
  limitCents: number; // monthly, home currency; 0 removes the budget
}
export interface DeleteBudgetRequest {
  categoryId: string;
}
export type DeleteBudgetResponse = Record<string, never>;
export interface ListBudgetsResponse {
  budgets: Budget[];
}
export interface SavingsGoal {
  id: string;
  name: string;
  targetCents: number;
  savedCents: number;
  deadline: Timestamp | null;
  monthlyNeededCents: number;
  /** Linked investment: savedCents follows its value. */
  investmentId: string;
  investmentName: string;
  /** Linked deposits: when the target is reached. */
  projectedAt: Timestamp | null;
}
export interface SetSavingsGoalRequest {
  id?: string;
  name: string;
  targetCents: number;
  savedCents: number;
  deadline?: Timestamp;
  investmentId?: string;
}
export interface DeleteSavingsGoalRequest {
  id: string;
}
export type DeleteSavingsGoalResponse = Record<string, never>;
export interface ListSavingsGoalsResponse {
  goals: SavingsGoal[];
}
export interface GetDashboardRequest {
  year: number;
  month: number;
}
export interface CategorySpend {
  categoryId: string;
  name: string;
  kind: CategoryKind;
  spentCents: number;
  budgetCents: number;
  budgetUsed: number;
}
export interface DailySpend {
  date: string;
  spentCents: number;
}
export interface MerchantSpend {
  merchant: string;
  spentCents: number;
  count: number;
}
export interface RuleBreakdown {
  needs: number;
  wants: number;
  savings: number;
}
export interface MonthTotal {
  month: string;
  incomeCents: number;
  expenseCents: number;
}
export interface Dashboard {
  currency: string;
  incomeCents: number;
  expenseCents: number;
  netCents: number;
  savingsRate: number;
  byCategory: CategorySpend[];
  daily: DailySpend[];
  topMerchants: MerchantSpend[];
  ruleBreakdown: RuleBreakdown | null; // actual 50/30/20 split
  lastSixMonths: MonthTotal[];
  projectedMonthExpenseCents: number;
  insights: string[];
  owedToMeCents: number;
  iOweCents: number;
}

export type RecurringPeriod =
  | 'RECURRING_PERIOD_UNSPECIFIED'
  | 'RECURRING_PERIOD_WEEKLY'
  | 'RECURRING_PERIOD_MONTHLY'
  | 'RECURRING_PERIOD_YEARLY';
export interface RecurringExpense {
  merchant: string;
  categoryId: string;
  averageCents: number; // home currency, positive
  currency: string; // home currency
  period: RecurringPeriod;
  occurrences: number;
  lastChargedAt: Timestamp | null;
  nextExpectedAt: Timestamp | null;
  yearlyCostCents: number;
  isSubscription: boolean;
}
export type ListRecurringRequest = Record<string, never>;
export interface ListRecurringResponse {
  items: RecurringExpense[];
  monthlyTotalCents: number;
  currency: string;
}

// ---------- notify.proto ----------
export interface RegisterDeviceRequest {
  token: string; // ExponentPushToken[...]
  platform: 'ios' | 'android' | 'web';
}
export type RegisterDeviceResponse = Record<string, never>;
export interface UnregisterDeviceRequest {
  token: string;
}
export type UnregisterDeviceResponse = Record<string, never>;
export type NotificationType =
  | 'friend_request'
  | 'friend_accepted'
  | 'group_joined'
  | 'receipt_parsed'
  | 'receipt_finalized'
  | 'expense_added'
  | 'settlement'
  | 'recurring_upcoming';
export interface AppNotification {
  id: string;
  type: NotificationType | string;
  title: string;
  body: string;
  /** e.g. {"receipt_id": "...", "group_id": "..."} — map keys are kept verbatim by the client. */
  data: Record<string, string>;
  read: boolean;
  createdAt: Timestamp | null;
}
export interface ListNotificationsRequest {
  limit: number;
  unreadOnly: boolean;
}
export interface ListNotificationsResponse {
  notifications: AppNotification[];
  unreadCount: number;
}
export interface MarkNotificationsReadRequest {
  ids: string[]; // empty — mark all
}
export type MarkNotificationsReadResponse = Record<string, never>;

export type Empty = Record<string, never>;

/** Error body returned by grpc-gateway on non-2xx. */
export interface ApiErrorBody {
  code: number;
  message: string;
}

// ---------- investments & market (finance.proto) ----------
export type InvestmentKind =
  | 'INVESTMENT_KIND_UNSPECIFIED'
  | 'INVESTMENT_KIND_DEPOSIT'
  | 'INVESTMENT_KIND_STOCK'
  | 'INVESTMENT_KIND_ETF'
  | 'INVESTMENT_KIND_CRYPTO'
  | 'INVESTMENT_KIND_BOND'
  | 'INVESTMENT_KIND_METAL'
  | 'INVESTMENT_KIND_OTHER';
export type Capitalization =
  | 'CAPITALIZATION_UNSPECIFIED'
  | 'CAPITALIZATION_MONTHLY'
  | 'CAPITALIZATION_QUARTERLY'
  | 'CAPITALIZATION_YEARLY'
  | 'CAPITALIZATION_NONE';
export interface Investment {
  id: string;
  kind: InvestmentKind;
  name: string;
  symbol: string;
  currency: string;
  quantity: number;
  investedCents: number;
  ratePercent: number;
  capitalization: Capitalization;
  monthlyContributionCents: number;
  openedAt: Timestamp | null;
  termMonths: number;
  valueCents: number;
  valueHomeCents: number;
  contributedCents: number;
  gainCents: number;
  gainPercent: number;
  price: number;
  dayChangePercent: number;
  effectiveRatePercent: number;
  maturityValueCents: number;
  maturityAt: Timestamp | null;
  quoteError: string;
}
export interface SetInvestmentRequest {
  id?: string;
  kind: InvestmentKind;
  name: string;
  symbol?: string;
  currency?: string;
  quantity?: number;
  investedCents: number;
  ratePercent?: number;
  capitalization?: Capitalization;
  monthlyContributionCents?: number;
  openedAt?: Timestamp;
  termMonths?: number;
}
export interface DeleteInvestmentRequest {
  id: string;
}
export interface AllocationSlice {
  kind: InvestmentKind;
  valueCents: number;
  share: number;
}
export interface PortfolioPoint {
  date: string;
  valueCents: number;
  contributedCents: number;
}
export interface Portfolio {
  currency: string;
  valueCents: number;
  contributedCents: number;
  gainCents: number;
  gainPercent: number;
  yearlyInterestCents: number;
  items: Investment[];
  allocation: AllocationSlice[];
  history: PortfolioPoint[];
}
export type MarketRange = '1mo' | '3mo' | '6mo' | '1y' | '5y';
export interface GetMarketChartRequest {
  symbol: string;
  range: MarketRange;
}
export interface MarketChart {
  symbol: string;
  name: string;
  currency: string;
  price: number;
  dayChangePercent: number;
  rangeChangePercent: number;
  points: { date: string; close: number }[];
  range: MarketRange;
}
export interface SymbolMatch {
  symbol: string;
  name: string;
  exchange: string;
  type: string;
}
export interface SearchSymbolsResponse {
  matches: SymbolMatch[];
}
export interface MarketQuote {
  symbol: string;
  name: string;
  currency: string;
  price: number;
  dayChangePercent: number;
  monthChangePercent: number;
  spark: number[];
  held: boolean;
}
export interface MarketOverview {
  quotes: MarketQuote[];
}

// ---------- net worth ----------
export type AccountKind =
  | 'ACCOUNT_KIND_UNSPECIFIED'
  | 'ACCOUNT_KIND_CASH'
  | 'ACCOUNT_KIND_CARD'
  | 'ACCOUNT_KIND_SAVINGS'
  | 'ACCOUNT_KIND_CREDIT_CARD'
  | 'ACCOUNT_KIND_LOAN'
  | 'ACCOUNT_KIND_PROPERTY';
export interface Account {
  id: string;
  name: string;
  kind: AccountKind;
  currency: string;
  balanceCents: number;
  balanceHomeCents: number;
  liability: boolean;
  liquid: boolean;
  updatedAt: Timestamp | null;
}
export interface ListAccountsResponse {
  accounts: Account[];
}
export interface SetAccountRequest {
  id?: string;
  name: string;
  kind: AccountKind;
  currency: string;
  balanceCents: number;
}
export interface NetWorthPoint {
  date: string;
  netCents: number;
  assetsCents: number;
  liabilitiesCents: number;
}
export interface NetWorth {
  currency: string;
  netCents: number;
  assetsCents: number;
  liabilitiesCents: number;
  cashCents: number;
  propertyCents: number;
  investmentsCents: number;
  owedToMeCents: number;
  creditCents: number;
  iOweCents: number;
  accounts: Account[];
  history: NetWorthPoint[];
  change30dCents: number;
  hasChange30d: boolean;
}

// ---------- month forecast ----------
export interface ForecastDay {
  date: string;
  netCents: number;
  actual: boolean;
}
export interface UpcomingCharge {
  merchant: string;
  amountCents: number;
  date: string;
}
export interface MonthForecast {
  currency: string;
  month: string;
  daysLeft: number;
  incomeSoFarCents: number;
  expenseSoFarCents: number;
  expectedIncomeCents: number;
  expectedExpenseCents: number;
  projectedNetCents: number;
  hasBalance: boolean;
  balanceNowCents: number;
  projectedBalanceCents: number;
  days: ForecastDay[];
  upcoming: UpcomingCharge[];
  historyMonths: number;
}

// ---------- export ----------
export type ExportKind = 'EXPORT_KIND_MONTH' | 'EXPORT_KIND_PORTFOLIO';
export type ExportFormat = 'EXPORT_FORMAT_CSV' | 'EXPORT_FORMAT_PDF';
export interface ExportReportRequest {
  kind: ExportKind;
  format: ExportFormat;
  year?: number;
  month?: number;
}
export interface ExportFile {
  filename: string;
  contentType: string;
  /** base64 */
  data: string;
}
