package domain

import "time"

// All money values are stored in minor units (cents) to avoid float rounding.

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Name         string
	Currency     string
	CreatedAt    time.Time
}

const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

type Group struct {
	ID        string
	Name      string
	OwnerID   string
	Currency  string
	Members   []GroupMember
	CreatedAt time.Time
}

type GroupMember struct {
	UserID string
	Name   string
	Email  string
	Role   string
}

type FriendStatus int16

const (
	FriendPendingIncoming FriendStatus = 1
	FriendPendingOutgoing FriendStatus = 2
	FriendAccepted        FriendStatus = 3
)

// Friend is a friendship seen from the current user's side.
type Friend struct {
	UserID string
	Name   string
	Email  string
	Status FriendStatus
	Since  time.Time
}

type InviteKind int16

const (
	InviteFriend InviteKind = 1
	InviteGroup  InviteKind = 2
)

type Invite struct {
	Token     string
	Kind      InviteKind
	InviterID string
	GroupID   string
	ExpiresAt time.Time
	MaxUses   int32
	Uses      int32
	Revoked   bool
	CreatedAt time.Time
}

func (i *Invite) Usable(now time.Time) bool {
	return !i.Revoked && now.Before(i.ExpiresAt) && (i.MaxUses == 0 || i.Uses < i.MaxUses)
}

func (g *Group) IsMember(userID string) bool {
	for _, m := range g.Members {
		if m.UserID == userID {
			return true
		}
	}
	return false
}

type ReceiptStatus int16

const (
	ReceiptPending   ReceiptStatus = 1
	ReceiptParsed    ReceiptStatus = 2
	ReceiptFailed    ReceiptStatus = 3
	ReceiptFinalized ReceiptStatus = 4
)

type Receipt struct {
	ID            string
	GroupID       string
	PayerID       string
	Store         string
	Status        ReceiptStatus
	Items         []ReceiptItem
	SubtotalCents int64
	TaxCents      int64
	TipCents      int64
	TotalCents    int64
	Currency      string
	ParseError    string
	OCRProvider   string
	PurchasedAt   *time.Time
	CreatedAt     time.Time
}

type ReceiptItem struct {
	ID         string
	Name       string
	RawName    string // as printed on the receipt
	Code       string // UPC/EAN
	Quantity   int32
	PriceCents int64
	Claims     []ItemClaim
}

type ItemClaim struct {
	UserID string
	Shares int32
}

type ReceiptFile struct {
	ReceiptID   string
	ContentType string
	Data        []byte
	RawText     string
}

type Debt struct {
	GroupID   string
	ReceiptID string
	FromUser  string // debtor
	ToUser    string // creditor
	Amount    int64  // group currency
	Currency  string
}

type Settlement struct {
	ID        string
	GroupID   string
	FromUser  string
	ToUser    string
	Amount    int64
	Currency  string
	CreatedAt time.Time
}

// ---------- Personal finance ----------

type CategoryKind int16

const (
	KindNeeds   CategoryKind = 1
	KindWants   CategoryKind = 2
	KindSavings CategoryKind = 3
	KindIncome  CategoryKind = 4
)

type Category struct {
	ID   string
	Name string
	Kind CategoryKind
	Icon string
}

type CategoryRule struct {
	Pattern    string
	CategoryID string
	UserID     string // empty — global rule
}

const (
	SourceManual    = "manual"
	SourceStatement = "statement"
	SourceReceipt   = "receipt"
)

type Transaction struct {
	ID          string
	UserID      string
	AmountCents int64 // <0 expense, >0 income
	Currency    string
	Description string
	Merchant    string
	CategoryID  string
	Source      string
	DedupKey    string
	OccurredAt  time.Time
}

type TransactionFilter struct {
	From, To   time.Time
	CategoryID string
	Limit      int
	Offset     int
}

const (
	ImportPending = "pending"
	ImportDone    = "done"
	ImportFailed  = "failed"
)

type StatementImport struct {
	ID         string
	UserID     string
	Status     string
	Format     int16
	Bank       string
	Currency   string
	File       []byte
	Imported   int
	Duplicates int
	Error      string
	CreatedAt  time.Time
}

type Budget struct {
	ID         string
	CategoryID string
	LimitCents int64
}

type SavingsGoal struct {
	ID          string
	UserID      string
	Name        string
	TargetCents int64
	SavedCents  int64
	Deadline    *time.Time
	// InvestmentID links the goal to an investment: progress follows its value.
	InvestmentID string
}

// Aggregates for dashboard (computed in SQL).

type CategoryTotal struct {
	Currency   string
	CategoryID string
	Name       string
	Kind       CategoryKind
	SpentCents int64 // positive number
}

type DailyTotal struct {
	Currency   string
	Date       time.Time
	SpentCents int64
}

type MerchantTotal struct {
	Currency   string
	Merchant   string
	SpentCents int64
	Count      int
}

type MonthTotal struct {
	Currency     string
	Month        time.Time
	IncomeCents  int64
	ExpenseCents int64
}

// CurrencyTotals is income/expense for one currency.
type CurrencyTotals struct {
	Currency     string
	IncomeCents  int64
	ExpenseCents int64
}

// Investment kinds (match the proto enum).
type InvestmentKind int

const (
	InvestDeposit InvestmentKind = iota + 1
	InvestStock
	InvestETF
	InvestCrypto
	InvestBond
	InvestMetal
	InvestOther
)

// Capitalization: how often deposit interest joins the principal.
type Capitalization int

const (
	CapMonthly Capitalization = iota + 1
	CapQuarterly
	CapYearly
	CapNone // simple interest
)

type Investment struct {
	ID                       string
	UserID                   string
	Kind                     InvestmentKind
	Name                     string
	Symbol                   string // market ticker; empty — valued by RatePercent
	Currency                 string
	Quantity                 float64
	InvestedCents            int64
	RatePercent              float64
	Capitalization           Capitalization
	MonthlyContributionCents int64
	OpenedAt                 time.Time
	TermMonths               int
}

// Market reports whether the investment is valued by a market quote.
func (i Investment) Market() bool { return i.Symbol != "" && i.Kind != InvestDeposit }

// Account kinds (match the proto enum).
type AccountKind int

const (
	AccountCash AccountKind = iota + 1
	AccountCard
	AccountSavings
	AccountCreditCard
	AccountLoan
	AccountProperty
)

// Liability: the balance is money owed.
func (k AccountKind) Liability() bool { return k == AccountCreditCard || k == AccountLoan }

// Liquid: money available to spend this month.
func (k AccountKind) Liquid() bool {
	return k == AccountCash || k == AccountCard || k == AccountSavings
}

// Account is a manually maintained balance used for net worth.
type Account struct {
	ID           string
	UserID       string
	Name         string
	Kind         AccountKind
	Currency     string
	BalanceCents int64 // >= 0; liabilities hold the amount owed
	UpdatedAt    time.Time
}

type NetWorthSnapshot struct {
	Day                           time.Time
	Currency                      string
	AssetsCents, LiabilitiesCents int64
}

// DailyFlow is income and spending of one day in one currency (transfers excluded).
type DailyFlow struct {
	Currency     string
	Date         time.Time
	IncomeCents  int64
	ExpenseCents int64
}
