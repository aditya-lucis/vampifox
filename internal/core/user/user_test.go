package user

import (
	"testing"
)

// ── Test: ValidatePassword ────────────────────────────────────────

func TestValidatePassword(t *testing.T) {
	valid := []string{
		"Password1",
		"Str0ngPass",
		"MyP4ssword",
		"abcDEF123",
		"12345678Aa",
	}
	for _, pw := range valid {
		if err := ValidatePassword(pw); err != nil {
			t.Errorf("ValidatePassword(%q) harus valid, got: %v", pw, err)
		}
	}

	invalid := []struct {
		pw   string
		desc string
	}{
		{"short1A", "terlalu pendek"},
		{"alllowercase1", "tidak ada huruf besar"},
		{"ALLUPPERCASE1", "tidak ada huruf kecil"},
		{"NoDigitsHere", "tidak ada angka"},
		{"", "kosong"},
	}
	for _, tt := range invalid {
		if err := ValidatePassword(tt.pw); err == nil {
			t.Errorf("ValidatePassword(%q) harus invalid (%s)", tt.pw, tt.desc)
		}
	}
}

// ── Test: isValidEmail ────────────────────────────────────────────

func TestIsValidEmail(t *testing.T) {
	valid := []string{
		"user@example.com",
		"user.name+tag@domain.co.id",
		"test123@mail.org",
		"a@b.io",
	}
	for _, e := range valid {
		if !isValidEmail(e) {
			t.Errorf("isValidEmail(%q) harus true", e)
		}
	}

	invalid := []string{
		"notanemail",
		"@nodomain.com",
		"noatsign",
		"spaces @domain.com",
		"",
	}
	for _, e := range invalid {
		if isValidEmail(e) {
			t.Errorf("isValidEmail(%q) harus false", e)
		}
	}
}

// ── Test: User.SetPassword & CheckPassword ────────────────────────

func TestUser_SetPassword_CheckPassword(t *testing.T) {
	u := &User{}
	plain := "Str0ngPassword"

	if err := u.SetPassword(plain, 4); err != nil { // cost 4 = cepat untuk test
		t.Fatalf("SetPassword() gagal: %v", err)
	}
	if u.PasswordHash == "" {
		t.Fatal("PasswordHash tidak boleh kosong setelah SetPassword")
	}
	if u.PasswordHash == plain {
		t.Fatal("PasswordHash tidak boleh sama dengan plain text")
	}

	// Password benar
	if err := u.CheckPassword(plain); err != nil {
		t.Errorf("CheckPassword() dengan password benar harus nil, got: %v", err)
	}

	// Password salah
	if err := u.CheckPassword("WrongPassword1"); err != ErrWrongPassword {
		t.Errorf("CheckPassword() password salah harus ErrWrongPassword, got: %v", err)
	}
}

func TestUser_SetPassword_WeakPassword(t *testing.T) {
	u := &User{}
	if err := u.SetPassword("weak", 4); err == nil {
		t.Error("SetPassword dengan password lemah harus error")
	}
}

// ── Test: User.IsActive ───────────────────────────────────────────

func TestUser_IsActive(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusActive, true},
		{StatusInactive, false},
		{StatusBanned, false},
	}
	for _, tt := range tests {
		u := &User{Status: tt.status}
		if got := u.IsActive(); got != tt.want {
			t.Errorf("status=%q IsActive()=%v, want %v", tt.status, got, tt.want)
		}
	}
}

// ── Test: User.HasRole, AddRole, RemoveRole ───────────────────────

func TestUser_RoleManagement(t *testing.T) {
	u := &User{}

	// Awalnya tidak punya role
	if u.HasRole("admin") {
		t.Error("user baru tidak seharusnya punya role")
	}

	// AddRole
	u.AddRole("familiar")
	u.AddRole("daywalker")
	if !u.HasRole("familiar") {
		t.Error("HasRole('familiar') harus true setelah AddRole")
	}
	if !u.HasRole("daywalker") {
		t.Error("HasRole('daywalker') harus true setelah AddRole")
	}

	// AddRole idempotent — tidak boleh duplikat
	u.AddRole("familiar")
	count := 0
	for _, r := range u.Roles {
		if r == "familiar" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("AddRole duplikat — familiar muncul %d kali, want 1", count)
	}

	// RemoveRole
	u.RemoveRole("familiar")
	if u.HasRole("familiar") {
		t.Error("HasRole('familiar') harus false setelah RemoveRole")
	}
	if !u.HasRole("daywalker") {
		t.Error("HasRole('daywalker') harus masih true setelah remove role lain")
	}

	// RemoveRole yang tidak ada — tidak panic
	u.RemoveRole("role-tidak-ada")
}

// ── Test: RegisterInput.Validate ─────────────────────────────────

func TestRegisterInput_Validate(t *testing.T) {
	valid := &RegisterInput{
		Email:    "user@example.com",
		Password: "Str0ngPass",
		FullName: "John Doe",
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid input harus tidak error, got: %v", err)
	}
	// Email harus di-lowercase
	if valid.Email != "user@example.com" {
		t.Errorf("Email harus lowercase, got: %q", valid.Email)
	}

	// Email invalid
	if err := (&RegisterInput{Email: "bukan-email", Password: "Str0ngPass", FullName: "Test"}).Validate(); err != ErrInvalidEmail {
		t.Errorf("email invalid harus ErrInvalidEmail, got: %v", err)
	}

	// Nama kosong
	if err := (&RegisterInput{Email: "a@b.com", Password: "Str0ngPass", FullName: ""}).Validate(); err == nil {
		t.Error("nama kosong harus error")
	}

	// Password lemah
	if err := (&RegisterInput{Email: "a@b.com", Password: "weak", FullName: "Test"}).Validate(); err == nil {
		t.Error("password lemah harus error")
	}
}

// ── Test: ChangePasswordInput.Validate ───────────────────────────

func TestChangePasswordInput_Validate(t *testing.T) {
	valid := &ChangePasswordInput{
		OldPassword: "OldPass1",
		NewPassword: "NewStr0ng",
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid input harus tidak error, got: %v", err)
	}

	// Old password kosong
	if err := (&ChangePasswordInput{OldPassword: "", NewPassword: "NewStr0ng"}).Validate(); err == nil {
		t.Error("old password kosong harus error")
	}

	// New == Old
	samePass := "SamePass1"
	if err := (&ChangePasswordInput{OldPassword: samePass, NewPassword: samePass}).Validate(); err == nil {
		t.Error("new password sama dengan old harus error")
	}

	// New password lemah
	if err := (&ChangePasswordInput{OldPassword: "OldPass1", NewPassword: "weak"}).Validate(); err == nil {
		t.Error("new password lemah harus error")
	}
}

// ── Test: UpdateProfileInput.Validate ────────────────────────────

func TestUpdateProfileInput_Validate(t *testing.T) {
	// Kosong semua — valid (update sebagian diperbolehkan)
	if err := (&UpdateProfileInput{}).Validate(); err != nil {
		t.Errorf("input kosong harus valid, got: %v", err)
	}

	// Nama terlalu pendek
	if err := (&UpdateProfileInput{FullName: "A"}).Validate(); err == nil {
		t.Error("nama 1 karakter harus error")
	}

	// Nama valid
	if err := (&UpdateProfileInput{FullName: "John"}).Validate(); err != nil {
		t.Errorf("nama valid harus tidak error, got: %v", err)
	}
}
