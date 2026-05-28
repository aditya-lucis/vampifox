// Package user — Manajemen user VampiFox.
//
// User selalu milik satu tenant — tidak ada user global.
// Satu orang bisa punya akun di banyak tenant (email sama, tapi
// record User-nya terpisah di schema masing-masing tenant).
//
// Password di-hash dengan bcrypt sebelum disimpan.
// Plain-text password tidak pernah menyentuh database.
package user

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ═══════════════════════════════════════════════════════════════
//  Sentinel errors
// ═══════════════════════════════════════════════════════════════

var (
	ErrNotFound       = errors.New("user tidak ditemukan")
	ErrEmailTaken     = errors.New("email sudah digunakan")
	ErrInvalidEmail   = errors.New("format email tidak valid")
	ErrWeakPassword   = errors.New("password terlalu lemah")
	ErrWrongPassword  = errors.New("password salah")
	ErrInactive       = errors.New("akun user tidak aktif")
)

// ═══════════════════════════════════════════════════════════════
//  Types
// ═══════════════════════════════════════════════════════════════

// Status kondisi akun user.
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
	StatusBanned   Status = "banned"
)

// ═══════════════════════════════════════════════════════════════
//  User model
// ═══════════════════════════════════════════════════════════════

// User merepresentasikan satu akun di dalam sebuah tenant.
// Disimpan di schema tenant (bukan schema sistem).
type User struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email        string     `gorm:"uniqueIndex;not null;size:255"`
	PasswordHash string     `gorm:"not null;size:255"`
	FullName     string     `gorm:"not null;size:255"`
	Avatar       string     `gorm:"size:500"`
	Status       Status     `gorm:"not null;default:'active'"`
	// Roles disimpan sebagai JSON array, e.g. ["elder_vampire", "daywalker"]
	Roles        []string   `gorm:"serializer:json"`
	// LastLoginAt nil artinya belum pernah login
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName — disimpan di schema tenant.
func (User) TableName() string { return "users" }

// ── Business logic methods ────────────────────────────────────────

// IsActive memeriksa apakah user bisa login.
func (u *User) IsActive() bool {
	return u.Status == StatusActive
}

// SetPassword meng-hash password dan menyimpannya ke PasswordHash.
// Plain-text password dibuang setelah fungsi ini selesai.
func (u *User) SetPassword(plain string, cost int) error {
	if err := ValidatePassword(plain); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword memverifikasi plain-text password terhadap hash.
// Mengembalikan ErrWrongPassword jika tidak cocok.
func (u *User) CheckPassword(plain string) error {
	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(plain))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return ErrWrongPassword
		}
		return err
	}
	return nil
}

// HasRole memeriksa apakah user memiliki role tertentu.
func (u *User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// AddRole menambahkan role jika belum ada (idempotent).
func (u *User) AddRole(role string) {
	if !u.HasRole(role) {
		u.Roles = append(u.Roles, role)
	}
}

// RemoveRole menghapus role jika ada.
func (u *User) RemoveRole(role string) {
	roles := u.Roles[:0]
	for _, r := range u.Roles {
		if r != role {
			roles = append(roles, r)
		}
	}
	u.Roles = roles
}

// ═══════════════════════════════════════════════════════════════
//  Input types
// ═══════════════════════════════════════════════════════════════

// RegisterInput data yang dibutuhkan untuk mendaftarkan user baru.
type RegisterInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

// Validate memvalidasi RegisterInput.
func (i *RegisterInput) Validate() error {
	i.Email = strings.ToLower(strings.TrimSpace(i.Email))
	i.FullName = strings.TrimSpace(i.FullName)

	if !isValidEmail(i.Email) {
		return ErrInvalidEmail
	}
	if strings.TrimSpace(i.FullName) == "" {
		return errors.New("nama lengkap wajib diisi")
	}
	if len(i.FullName) < 2 {
		return errors.New("nama lengkap minimal 2 karakter")
	}
	return ValidatePassword(i.Password)
}

// UpdateProfileInput data untuk update profil user.
type UpdateProfileInput struct {
	FullName string `json:"full_name"`
	Avatar   string `json:"avatar"`
}

// Validate memvalidasi UpdateProfileInput.
func (i *UpdateProfileInput) Validate() error {
	i.FullName = strings.TrimSpace(i.FullName)
	if i.FullName != "" && len(i.FullName) < 2 {
		return errors.New("nama lengkap minimal 2 karakter")
	}
	return nil
}

// ChangePasswordInput data untuk ganti password.
type ChangePasswordInput struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// Validate memvalidasi ChangePasswordInput.
func (i *ChangePasswordInput) Validate() error {
	if i.OldPassword == "" {
		return errors.New("password lama wajib diisi")
	}
	if i.OldPassword == i.NewPassword {
		return errors.New("password baru tidak boleh sama dengan password lama")
	}
	return ValidatePassword(i.NewPassword)
}

// ═══════════════════════════════════════════════════════════════
//  Validation helpers
// ═══════════════════════════════════════════════════════════════

// emailPattern untuk validasi format email.
var emailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// isValidEmail memeriksa format email.
func isValidEmail(email string) bool {
	return emailPattern.MatchString(email)
}

// ValidatePassword memeriksa kekuatan password.
//
// Syarat:
//   - Minimal 8 karakter
//   - Minimal 1 huruf besar
//   - Minimal 1 huruf kecil
//   - Minimal 1 angka
//
// Sengaja tidak memaksa karakter spesial — terlalu restrictive
// dan terbukti tidak meningkatkan keamanan secara signifikan.
func ValidatePassword(pw string) error {
	if len(pw) < 8 {
		return errors.New("password minimal 8 karakter")
	}

	var hasUpper, hasLower, hasDigit bool
	for _, c := range pw {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasDigit = true
		}
	}

	var missing []string
	if !hasUpper {
		missing = append(missing, "huruf besar")
	}
	if !hasLower {
		missing = append(missing, "huruf kecil")
	}
	if !hasDigit {
		missing = append(missing, "angka")
	}

	if len(missing) > 0 {
		return errors.New("password harus mengandung: " + strings.Join(missing, ", "))
	}

	return nil
}
