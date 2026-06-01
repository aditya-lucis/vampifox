package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// AccountType jenis akun dalam Chart of Accounts.
type AccountType string

const (
	AccountTypeAsset     AccountType = "asset"
	AccountTypeLiability AccountType = "liability"
	AccountTypeEquity    AccountType = "equity"
	AccountTypeRevenue   AccountType = "revenue"
	AccountTypeExpense   AccountType = "expense"
)

// AccountNormalBalance saldo normal akun.
type AccountNormalBalance string

const (
	NormalBalanceDebit  AccountNormalBalance = "debit"
	NormalBalanceCredit AccountNormalBalance = "credit"
)

// NormalBalanceFor mengembalikan saldo normal berdasarkan tipe akun.
func NormalBalanceFor(t AccountType) AccountNormalBalance {
	switch t {
	case AccountTypeAsset, AccountTypeExpense:
		return NormalBalanceDebit
	default:
		return NormalBalanceCredit
	}
}

// Account adalah satu akun dalam Chart of Accounts.
type Account struct {
	ID            uuid.UUID            `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Code          string               `gorm:"uniqueIndex;not null;size:20"`
	Name          string               `gorm:"not null;size:255"`
	Type          AccountType          `gorm:"not null"`
	NormalBalance AccountNormalBalance `gorm:"not null"`
	ParentID      *uuid.UUID           `gorm:"type:uuid;index"`
	IsHeader      bool                 `gorm:"default:false"`
	IsActive      bool                 `gorm:"default:true"`
	Description   string               `gorm:"size:500"`
	Balance       float64              `gorm:"default:0;type:decimal(20,4)"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (Account) TableName() string { return "acc_accounts" }

var (
	ErrAccountNotFound  = errors.New("akun tidak ditemukan")
	ErrAccountCodeTaken = errors.New("kode akun sudah digunakan")
	ErrAccountIsHeader  = errors.New("akun header tidak bisa diposting")
	ErrAccountInactive  = errors.New("akun tidak aktif")
)

func (a *Account) Validate() error {
	if a.Code == "" {
		return errors.New("kode akun wajib diisi")
	}
	if a.Name == "" {
		return errors.New("nama akun wajib diisi")
	}
	switch a.Type {
	case AccountTypeAsset, AccountTypeLiability, AccountTypeEquity,
		AccountTypeRevenue, AccountTypeExpense:
	default:
		return errors.New("tipe akun tidak valid")
	}
	return nil
}

func (a *Account) CanPost() error {
	if a.IsHeader {
		return ErrAccountIsHeader
	}
	if !a.IsActive {
		return ErrAccountInactive
	}
	return nil
}

// StandardCOA mengembalikan Chart of Accounts standar.
func StandardCOA() []Account {
	return []Account{
		{Code: "1-0000", Name: "ASET", Type: AccountTypeAsset, NormalBalance: NormalBalanceDebit, IsHeader: true},
		{Code: "1-1000", Name: "Aset Lancar", Type: AccountTypeAsset, NormalBalance: NormalBalanceDebit, IsHeader: true},
		{Code: "1-1001", Name: "Kas", Type: AccountTypeAsset, NormalBalance: NormalBalanceDebit},
		{Code: "1-1002", Name: "Bank", Type: AccountTypeAsset, NormalBalance: NormalBalanceDebit},
		{Code: "1-1100", Name: "Piutang Usaha", Type: AccountTypeAsset, NormalBalance: NormalBalanceDebit},
		{Code: "1-1200", Name: "Persediaan", Type: AccountTypeAsset, NormalBalance: NormalBalanceDebit},
		{Code: "1-2000", Name: "Aset Tetap", Type: AccountTypeAsset, NormalBalance: NormalBalanceDebit, IsHeader: true},
		{Code: "1-2001", Name: "Peralatan", Type: AccountTypeAsset, NormalBalance: NormalBalanceDebit},
		{Code: "1-2002", Name: "Akum. Penyusutan Peralatan", Type: AccountTypeAsset, NormalBalance: NormalBalanceCredit},
		{Code: "2-0000", Name: "KEWAJIBAN", Type: AccountTypeLiability, NormalBalance: NormalBalanceCredit, IsHeader: true},
		{Code: "2-1000", Name: "Kewajiban Jangka Pendek", Type: AccountTypeLiability, NormalBalance: NormalBalanceCredit, IsHeader: true},
		{Code: "2-1001", Name: "Utang Usaha", Type: AccountTypeLiability, NormalBalance: NormalBalanceCredit},
		{Code: "2-1002", Name: "Utang Pajak", Type: AccountTypeLiability, NormalBalance: NormalBalanceCredit},
		{Code: "3-0000", Name: "MODAL", Type: AccountTypeEquity, NormalBalance: NormalBalanceCredit, IsHeader: true},
		{Code: "3-1001", Name: "Modal Disetor", Type: AccountTypeEquity, NormalBalance: NormalBalanceCredit},
		{Code: "3-1002", Name: "Laba Ditahan", Type: AccountTypeEquity, NormalBalance: NormalBalanceCredit},
		{Code: "4-0000", Name: "PENDAPATAN", Type: AccountTypeRevenue, NormalBalance: NormalBalanceCredit, IsHeader: true},
		{Code: "4-1001", Name: "Pendapatan Usaha", Type: AccountTypeRevenue, NormalBalance: NormalBalanceCredit},
		{Code: "4-1002", Name: "Pendapatan Lain-lain", Type: AccountTypeRevenue, NormalBalance: NormalBalanceCredit},
		{Code: "5-0000", Name: "BEBAN", Type: AccountTypeExpense, NormalBalance: NormalBalanceDebit, IsHeader: true},
		{Code: "5-1001", Name: "Beban Pokok Penjualan", Type: AccountTypeExpense, NormalBalance: NormalBalanceDebit},
		{Code: "5-2001", Name: "Beban Gaji", Type: AccountTypeExpense, NormalBalance: NormalBalanceDebit},
		{Code: "5-2002", Name: "Beban Sewa", Type: AccountTypeExpense, NormalBalance: NormalBalanceDebit},
		{Code: "5-2003", Name: "Beban Utilitas", Type: AccountTypeExpense, NormalBalance: NormalBalanceDebit},
		{Code: "5-2004", Name: "Beban Penyusutan", Type: AccountTypeExpense, NormalBalance: NormalBalanceDebit},
	}
}
