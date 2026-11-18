package statement

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// All fixtures are synthetic: they mimic the bank layouts with invented data.

func loadPagesFile(t *testing.T, name string) [][]string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var pages [][]string
	for _, p := range strings.Split(string(b), "\f\n") {
		pages = append(pages, strings.Split(strings.TrimSuffix(p, "\n"), "\n"))
	}
	return pages
}

type kzWant struct {
	date     string
	cents    int64
	category string
	merchant string
	desc     string
}

func checkRows(t *testing.T, rows []Row, want []kzWant, currency string) {
	t.Helper()
	if len(rows) != len(want) {
		for _, r := range rows {
			t.Logf("%s %d %q %q %q", r.Date.Format("2006-01-02"), r.AmountCents, r.Category, r.Merchant, r.Description)
		}
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	keys := map[string]bool{}
	for i, w := range want {
		r := rows[i]
		if r.Date.Format("2006-01-02") != w.date || r.AmountCents != w.cents || r.Category != w.category ||
			(w.merchant != "" && r.Merchant != w.merchant) || r.Description != w.desc || r.Currency != currency {
			t.Errorf("row %d = {%s %d %s %q %q %q}, want %+v", i, r.Date.Format("2006-01-02"), r.AmountCents,
				r.Currency, r.Category, r.Merchant, r.Description, w)
		}
		if r.DedupKey == "" || keys[r.DedupKey] {
			t.Errorf("row %d: empty or duplicate dedup key", i)
		}
		keys[r.DedupKey] = true
	}
}

func sums(rows []Row) (in, out int64) {
	for _, r := range rows {
		if r.AmountCents > 0 {
			in += r.AmountCents
		} else {
			out -= r.AmountCents
		}
	}
	return
}

// ---------- Bank CenterCredit ----------

func bccWant(lang string) []kzWant {
	tr := map[string][2]string{
		"Transfer":    {"Transfer", "Перевод"},
		"Purchase":    {"Purchase", "Покупка"},
		"TransferTo":  {"Transfer to Иван И.", "Перевод Иван И."},
		"Replenish":   {"Replenishment", "Пополнение"},
		"Replenished": {"Пополнение", "Пополнение"},
	}
	l := 0
	if lang == "ru" {
		l = 1
	}
	return []kzWant{
		{"2026-03-30", -200000, "transfer", "", tr["Transfer"][l]},
		{"2026-03-28", -501250, "", "COFFEE POINT", tr["Purchase"][l] + " COFFEE POINT"},
		// The name wrapped past the amounts and over the page break.
		{"2026-03-25", -350000, "transfer", "ИВАН И.", tr["TransferTo"][l]},
		// Annual service fee: zero amount, the fee is the expense.
		{"2026-03-20", -500000, "", "", "BO. Годовое обслуживание счета."},
		// Fee added to the purchase; cashback ignored; amount split mid-number.
		{"2026-03-15", -5015800, "", "SUPER STORE", tr["Purchase"][l] + " SUPER STORE"},
		{"2026-03-10", 20000000, "transfer", "", tr["Replenished"][l]},
		{"2026-03-05", -120000, "", "MAGNUM", tr["Purchase"][l] + " MAGNUM"},
		{"2026-03-05", -120000, "", "MAGNUM", tr["Purchase"][l] + " MAGNUM"},
		{"2026-03-02", 5000000, "transfer", "", tr["Replenish"][l]},
	}
}

func TestBCC(t *testing.T) {
	en, ru := loadPagesFile(t, "bcc_en_pages.txt"), loadPagesFile(t, "bcc_ru_pages.txt")
	var keys []string
	for _, c := range []struct {
		lang  string
		pages [][]string
	}{{"en", en}, {"ru", ru}} {
		if b := DetectBank(c.pages); b != BankBCC {
			t.Fatalf("%s: detected %q", c.lang, b)
		}
		rows, err := ParsePages(c.pages)
		if err != nil {
			t.Fatal(err)
		}
		checkRows(t, rows, bccWant(c.lang), "KZT")
		// Reconciles with "Incomings / Expenses for the period" of the header.
		if in, out := sums(rows); in != 25000000 || out != 6807050 {
			t.Errorf("%s: incomings %d, expenses %d", c.lang, in, out)
		}
		if keys == nil {
			for _, r := range rows {
				keys = append(keys, r.DedupKey)
			}
			continue
		}
		// The English and the Russian version of a statement deduplicate.
		for i, r := range rows {
			if r.DedupKey != keys[i] {
				t.Errorf("row %d: ru key differs from en", i)
			}
		}
	}
}

func TestBCCAccountCurrency(t *testing.T) {
	pages := [][]string{{"Bank CenterCredit JSC", "Account currency USD",
		"2026-03-30 2026-03-30 Purchase SHOP 10.00 USD -10.00 USD 0.00 USD 0.00USD"}}
	rows, err := ParsePages(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Currency != "USD" || rows[0].AmountCents != -1000 || rows[0].Description != "Purchase SHOP" {
		t.Errorf("rows = %+v", rows)
	}
}

// ---------- Kaspi (layout reproduced without a real sample) ----------

const kaspiRU = `АО «Kaspi Bank», БИК CASPKZKA, www.kaspi.kz
ВЫПИСКА
по Kaspi Gold за период с 01.10.25 по 31.10.25
Тестов Тест
Номер карты: *0000
Валюта счета: тенге
Доступно на 31.10.25 +11 345,67 ₸
Краткое содержание операций по карте:
Доступно на 01.10.25 +1 000,00 ₸
Пополнения + 100 000,00 ₸
Переводы - 30 000,00 ₸
Покупки - 56 654,33 ₸
Снятия - 3 000,00 ₸
Разное + 0,00 ₸
Доступно на 31.10.25 +11 345,67 ₸
Дата Сумма Операция Детали
27.10.25 - 1 500,00 ₸ Покупки MAGNUM CASH&CARRY
26.10.25 + 50 000,00 ₸ Пополнения С Kaspi Депозита
25.10.25 - 30 000,00 ₸ Переводы Иван И.
24.10.25 - 3 000,00 ₸ Снятия Банкомат Алматы
` + "\f\n" + `АО «Kaspi Bank», БИК CASPKZKA, www.kaspi.kz
Дата Сумма Операция Детали
23.10.25 - 5 154,33 ₸ Покупки APPLE.COM/BILL
22.10.25 - 50 000,00 ₸ Покупки ИП Тестовый магазин
заказ 12
21.10.25 + 50 000,00 ₸ Пополнения С карты другого банка
21.10.25 + 50 000,00 ₸ Пополнения С карты другого банка
21.10.25 - 50 000,00 ₸ Разное Возврат пополнения
`

func pagesOf(s string) [][]string {
	var pages [][]string
	for _, p := range strings.Split(s, "\f\n") {
		pages = append(pages, strings.Split(strings.TrimSuffix(p, "\n"), "\n"))
	}
	return pages
}

func TestKaspiRU(t *testing.T) {
	pages := pagesOf(kaspiRU)
	if b := DetectBank(pages); b != BankKaspi {
		t.Fatalf("detected %q", b)
	}
	rows, err := ParsePages(pages)
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, rows, []kzWant{
		{"2025-10-27", -150000, "", "MAGNUM CASH&CARRY", "MAGNUM CASH&CARRY"},
		{"2025-10-26", 5000000, "transfer", "", "С Kaspi Депозита"},
		{"2025-10-25", -3000000, "transfer", "ИВАН И.", "Иван И."},
		{"2025-10-24", -300000, "transfer", "", "Банкомат Алматы"},
		{"2025-10-23", -515433, "", "APPLE.COM/BILL", "APPLE.COM/BILL"},
		{"2025-10-22", -5000000, "", "", "ИП Тестовый магазин заказ 12"},
		{"2025-10-21", 5000000, "transfer", "", "С карты другого банка"},
		{"2025-10-21", 5000000, "transfer", "", "С карты другого банка"},
		{"2025-10-21", -5000000, "", "", "Возврат пополнения"},
	}, "KZT")
}

func TestKaspiEN(t *testing.T) {
	pages := [][]string{{
		"Kaspi Bank JSC, BIC CASPKZKA, www.kaspi.kz",
		"STATEMENT",
		"Kaspi Gold for the period from 01.10.25 to 31.10.25",
		"Date Amount Transaction Details",
		"27.10.2025  - 1 500,00 ₸  Purchases  SMALL MARKET",
		"26.10.25 + 20 000,00 ₸ Replenishment From Kaspi Deposit",
		"25.10.25 - 2 000,00 ₸ Transfers Anna A.",
		"24.10.25 - 10 000,00 ₸ Withdrawals ATM Astana",
		"23.10.25 - 990,00 ₸ Others Fee",
	}}
	rows, err := ParsePages(pages)
	if err != nil {
		t.Fatal(err)
	}
	checkRows(t, rows, []kzWant{
		{"2025-10-27", -150000, "", "SMALL MARKET", "SMALL MARKET"},
		{"2025-10-26", 2000000, "transfer", "", "From Kaspi Deposit"},
		{"2025-10-25", -200000, "transfer", "", "Anna A."},
		{"2025-10-24", -1000000, "transfer", "", "ATM Astana"},
		{"2025-10-23", -99000, "", "", "Fee"},
	}, "KZT")
}

// ---------- generic ----------

func TestGenericRussianSignedWithBalance(t *testing.T) {
	// Signed amounts and a balance column; summary lines are skipped.
	pages := [][]string{{
		"АО «Какой-то Банк»",
		"Выписка по счету за период 01.10.2025 - 31.10.2025",
		"Входящий остаток: 100 000,00 ₸",
		"Дата операции Описание Сумма Остаток",
		"01.10.2025 Магазин ромашка -1 500,50 ₸ 98 499,50 ₸",
		"03.10.2025 14:05 Зарплата +250 000,00 ₸ 348 499,50 ₸",
		"05.10.2025 Такси ЯНДЕКС",
		"поездка по городу -2 000,00 ₸ 346 499,50 ₸",
		"Итого поступлений 250 000,00 ₸",
		"Исходящий остаток 346 499,50 ₸",
		"Страница 1 из 1",
	}}
	if b := DetectBank(pages); b != BankGeneric {
		t.Fatalf("detected %q", b)
	}
	rows, err := ParsePages(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	want := []struct {
		date  string
		cents int64
		desc  string
	}{
		{"2025-10-01", -150050, "Магазин ромашка"},
		{"2025-10-03", 25000000, "Зарплата"},
		{"2025-10-05", -200000, "Такси ЯНДЕКС поездка по городу"},
	}
	for i, w := range want {
		r := rows[i]
		if r.Date.Format("2006-01-02") != w.date || r.AmountCents != w.cents || r.Description != w.desc || r.Currency != "KZT" {
			t.Errorf("row %d = %+v, want %+v", i, r, w)
		}
	}
}

func TestGenericEuropeanDebitCreditBalance(t *testing.T) {
	// "1.234,56" amounts, unsigned, separate debit/credit columns and a running
	// balance: the sign comes from the balance movement.
	pages := [][]string{{
		"Example Bank AG — Account statement",
		"Date Description Debit Credit Balance",
		"02.09.2025 Opening balance 1.000,00",
		"03.09.2025 REWE MARKT BERLIN 45,90 954,10",
		"04.09.2025 Gehalt ACME GmbH 2.500,00 3.454,10",
		"05.09.2025 Miete September 1.200,00 2.254,10",
		"06.09.2025 Refund REWE 0,00 5,00 2.259,10",
		"Closing balance 2.259,10",
	}}
	rows, err := ParsePages(pages)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, r := range rows {
		got[r.Description] = r.AmountCents
	}
	want := map[string]int64{"REWE MARKT BERLIN": -4590, "Gehalt ACME GmbH": 250000, "Miete September": -120000, "Refund REWE": 500}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for d, c := range want {
		if got[d] != c {
			t.Errorf("%q = %d, want %d", d, got[d], c)
		}
	}
}

func TestGenericUSAndTextDates(t *testing.T) {
	pages := [][]string{{
		"Date Description Amount",
		"10/27/2025 STARBUCKS STORE 123 -$4.50",
		"Oct 28, 2025 PAYROLL ACME CORP $1,234.56 Cr",
		"10/29/2025 RENT PAYMENT (1,200.00)",
	}}
	rows, err := ParsePages(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Date.Format("2006-01-02") != "2025-10-27" || rows[0].AmountCents != -450 ||
		rows[0].Currency != "USD" || rows[1].AmountCents != 123456 || rows[2].AmountCents != -120000 ||
		rows[1].Description != "PAYROLL ACME CORP" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestGenericRussianTextDatesUnsignedKeywords(t *testing.T) {
	pages := [][]string{{
		"27 окт. 2025 Оплата кофейня 1 200,00 KZT",
		"28 октября 2025 г. Пополнение карты 10 000,00 KZT",
		"29 Oct 2025 Cinema 3 500,00 KZT",
	}}
	rows, err := ParsePages(pages)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].AmountCents != -120000 || rows[1].AmountCents != 1000000 ||
		rows[2].AmountCents != -350000 || rows[1].Date.Format("2006-01-02") != "2025-10-28" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestGenericCSVSemicolon(t *testing.T) {
	data := []byte("\xef\xbb\xbfДата;Описание;Сумма;Остаток\n" +
		"27.10.2025;Магазин;-1 500,00;8 500,00\n" +
		"28.10.2025;\"Перевод от Ивана; спасибо\";2 000,00;10 500,00\n")
	rows, err := Parse(data, FormatAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].AmountCents != -150000 || rows[1].AmountCents != 200000 ||
		rows[1].Description != "Перевод от Ивана; спасибо" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestGenericUnrecognized(t *testing.T) {
	for _, pages := range [][][]string{
		{{"Other Bank", "hello"}},
		{{"Some report", "Total 1 000,00", "Balance 2,000.00"}},
		{},
	} {
		_, err := ParsePages(pages)
		if !errors.Is(err, ErrUnrecognized) || !errors.Is(err, ErrUnsupported) {
			t.Errorf("%v: err = %v", pages, err)
		}
	}
	if _, err := Parse([]byte("foo,bar\nbaz,qux\n"), FormatCSV); !errors.Is(err, ErrUnrecognized) {
		t.Errorf("csv err = %v", err)
	}
	if !strings.Contains(ErrUnrecognized.Error(), "пришлите пример файла") {
		t.Error("message")
	}
}

// ---------- detection ----------

func TestDetectBankOrder(t *testing.T) {
	cases := []struct {
		name  string
		pages [][]string
		want  Bank
	}{
		{"freedom", [][]string{{"JSC \"Freedom Bank Kazakhstan\"", "www.bankffin.kz"}}, BankFreedom},
		{"freedom table header", [][]string{{"x"}, {"Date The amount Currency Operation Details"}}, BankFreedom},
		{"bcc", [][]string{{"Bank CenterCredit JSC", "Web: www.bcc.kz"}}, BankBCC},
		{"bcc ru", [][]string{{"АКЦИОНЕРНОЕ ОБЩЕСТВО «БАНК ЦЕНТРКРЕДИТ»"}}, BankBCC},
		// A bank named in a transaction description doesn't change the layout.
		{"bcc mentioning freedom", [][]string{{"Bank CenterCredit JSC", "2026-01-01 2026-01-01 Transfer to Freedom Bank"}}, BankBCC},
		{"kaspi", [][]string{{"АО «Kaspi Bank», БИК CASPKZKA, www.kaspi.kz"}}, BankKaspi},
		{"kaspi table header", [][]string{{"x"}, {"Дата  Сумма  Операция  Детали"}}, BankKaspi},
		{"freedom before kaspi", [][]string{{"Freedom Bank", "kaspi.kz"}}, BankFreedom},
		{"bcc before kaspi", [][]string{{"bcc.kz", "Kaspi Gold"}}, BankBCC},
		{"generic", [][]string{{"Halyk Bank", "27.10.2025 Coffee -1 000,00"}}, BankGeneric},
	}
	for _, c := range cases {
		if got := DetectBank(c.pages); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	// A detected bank whose parser finds nothing falls back to the generic parser.
	rows, err := ParsePages([][]string{{"www.kaspi.kz", "27.10.2025 Coffee 1 000,00 ₸ x"}})
	if err != nil || len(rows) != 1 {
		t.Errorf("fallback: %+v, %v", rows, err)
	}
}

func TestParseNumber(t *testing.T) {
	cases := map[string]int64{"1 234,56": 123456, "1,234.56": 123456, "1.234,56": 123456, "1234.5": 123450,
		"1,234": 123400, "1.234.567": 123456700, "0,07": 7, "12": 1200, "1 500": 150000}
	for in, want := range cases {
		if got, _, ok := parseNumber(in); !ok || got != want {
			t.Errorf("parseNumber(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
}

func TestFindMoney(t *testing.T) {
	cases := []struct {
		in   string
		want []int64 // signed with an expense default
	}{
		{"Coffee 4.50", []int64{-450}},
		{"Shop #1234 card 4400 total 12,00 ₸", []int64{-1200}},
		{"(1,200.00) and +5 000,00 ₸", []int64{-120000, 500000}},
		{"−1 500,00 RUB", []int64{-150000}},
		{"1.234,56- 99,00 Cr", []int64{-123456, 9900}},
		{"card 400000******1111 at 10:30", nil},
	}
	for _, c := range cases {
		ms := findMoney(c.in)
		var got []int64
		for _, m := range ms {
			got = append(got, m.signed(-1))
		}
		if len(got) != len(c.want) {
			t.Errorf("%q: %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q: %v, want %v", c.in, got, c.want)
			}
		}
	}
}
