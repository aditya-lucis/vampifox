package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ═══════════════════════════════════════════════════════════════
//  Journal Entry — Jurnal Akuntansi
// ═══════════════════════════════════════════════════════════════

// JournalStatus status jurnal.
type JournalStatus string

const (
	JournalStatusDraft  JournalStatus = "draft"
	JournalStatusPosted JournalStatus = "posted"
	JournalStatusVoided JournalStatus = "voided"
)

// Journal adalah satu jurnal entry (header).
// Satu Journal punya banyak JournalLine (debit/kredit).
// Aturan: total debit HARUS sama dengan total kredit.
type Journal struct {
	ID          uuid.UUID     `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Number      string        `gorm:"uniqueIndex;not null;size:50"` // e.g. "JRN-2024-0001"
	Date        time.Time     `gorm:"not null;index"`
	Description string        `gorm:"not null;size:500"`
	Reference   string        `gorm:"size:100"` // referensi dokumen (invoice, SO, dll)
	Status      JournalStatus `gorm:"not null;default:'draft'"`
	TotalDebit  float64       `gorm:"default:0;type:decimal(20,4)"`
	TotalCredit float64       `gorm:"default:0;type:decimal(20,4)"`
	// CreatedBy adalah user UUID yang membuat jurnal
	CreatedBy  uuid.UUID  `gorm:"type:uuid;not null"`
	PostedBy   *uuid.UUID `gorm:"type:uuid"`
	PostedAt   *time.Time
	VoidedBy   *uuid.UUID `gorm:"type:uuid"`
	VoidedAt   *time.Time
	VoidReason string `gorm:"size:500"`
	CreatedAt  time.Time
	UpdatedAt  time.Time

	// Preload
	Lines []JournalLine `gorm:"foreignKey:JournalID"`
}

func (Journal) TableName() string { return "acc_journals" }

// JournalLine adalah satu baris dalam jurnal (debit atau kredit).
type JournalLine struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	JournalID   uuid.UUID `gorm:"type:uuid;not null;index"`
	AccountID   uuid.UUID `gorm:"type:uuid;not null;index"`
	AccountCode string    `gorm:"size:20"` // denormalisasi untuk performa
	AccountName string    `gorm:"size:255"`
	Description string    `gorm:"size:500"`
	Debit       float64   `gorm:"default:0;type:decimal(20,4)"`
	Credit      float64   `gorm:"default:0;type:decimal(20,4)"`
	Sequence    int       `gorm:"default:0"` // urutan baris

	// Preload
	Account *Account `gorm:"foreignKey:AccountID"`
}

func (JournalLine) TableName() string { return "acc_journal_lines" }

// ── Errors ────────────────────────────────────────────────────────

var (
	ErrJournalNotFound      = errors.New("jurnal tidak ditemukan")
	ErrJournalNotBalanced   = errors.New("jurnal tidak balance: total debit harus sama dengan total kredit")
	ErrJournalAlreadyPosted = errors.New("jurnal sudah diposting, tidak bisa diubah")
	ErrJournalAlreadyVoided = errors.New("jurnal sudah dibatalkan")
	ErrJournalNoLines       = errors.New("jurnal harus punya minimal 2 baris")
)

// ── Business logic ────────────────────────────────────────────────

// IsBalanced memeriksa apakah total debit sama dengan total kredit.
func (j *Journal) IsBalanced() bool {
	return roundCurrency(j.TotalDebit) == roundCurrency(j.TotalCredit)
}

// Validate memeriksa jurnal sebelum diposting.
func (j *Journal) Validate() error {
	if len(j.Lines) < 2 {
		return ErrJournalNoLines
	}

	var totalDebit, totalCredit float64
	for _, line := range j.Lines {
		if line.Debit < 0 || line.Credit < 0 {
			return errors.New("nilai debit/kredit tidak boleh negatif")
		}
		if line.Debit == 0 && line.Credit == 0 {
			return errors.New("setiap baris harus punya nilai debit atau kredit")
		}
		if line.Debit > 0 && line.Credit > 0 {
			return errors.New("satu baris tidak boleh punya nilai debit DAN kredit sekaligus")
		}
		totalDebit += line.Debit
		totalCredit += line.Credit
	}

	j.TotalDebit = totalDebit
	j.TotalCredit = totalCredit

	if !j.IsBalanced() {
		return fmt.Errorf("%w: debit=%.4f, kredit=%.4f",
			ErrJournalNotBalanced, totalDebit, totalCredit)
	}
	return nil
}

// CanPost memeriksa apakah jurnal bisa diposting.
func (j *Journal) CanPost() error {
	switch j.Status {
	case JournalStatusPosted:
		return ErrJournalAlreadyPosted
	case JournalStatusVoided:
		return ErrJournalAlreadyVoided
	}
	return j.Validate()
}

// CanVoid memeriksa apakah jurnal bisa dibatalkan.
func (j *Journal) CanVoid() error {
	if j.Status == JournalStatusVoided {
		return ErrJournalAlreadyVoided
	}
	if j.Status == JournalStatusDraft {
		return errors.New("jurnal draft tidak perlu dibatalkan, hapus saja")
	}
	return nil
}

// roundCurrency membulatkan angka ke 4 desimal untuk perbandingan.
func roundCurrency(v float64) float64 {
	return float64(int64(v*10000+0.5)) / 10000
}
