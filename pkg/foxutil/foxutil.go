// Package foxutil — Utilitas umum VampiFox.
// "Fox" dalam VampiFox: cerdik, adaptif, dan selalu punya trik.
// Kumpulan helper yang digunakan di seluruh codebase.
package foxutil

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Slugify mengubah string menjadi URL-safe slug.
// "PT Maju Jaya Tbk." → "pt-maju-jaya-tbk"
func Slugify(s string) string {
	s = strings.ToLower(s)
	// Ganti karakter non-alphanumeric dengan dash
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// GenerateToken membuat random token yang aman secara kriptografis.
func GenerateToken(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("foxutil: gagal generate token: %w", err)
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// NightTimestamp menghasilkan timestamp dengan timezone Jakarta.
// VampiFox beroperasi di waktu lokal Indonesia.
func NightTimestamp() time.Time {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	return time.Now().In(loc)
}

// MaskEmail menyembunyikan sebagian email untuk keamanan log.
// "user@example.com" → "us**@example.com"
func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***"
	}
	local := parts[0]
	if len(local) <= 2 {
		return "**@" + parts[1]
	}
	return local[:2] + strings.Repeat("*", len(local)-2) + "@" + parts[1]
}

// IsStrongPassword memvalidasi kekuatan password.
// VampiFox tidak menerima password lemah — layaknya vampire yang tidak bisa dibunuh sembarangan.
func IsStrongPassword(pw string) bool {
	if len(pw) < 8 {
		return false
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, c := range pw {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasDigit = true
		case unicode.IsPunct(c) || unicode.IsSymbol(c):
			hasSpecial = true
		}
	}
	return hasUpper && hasLower && hasDigit && hasSpecial
}

// Paginate menghitung offset untuk pagination SQL.
func Paginate(page, pageSize int) (limit, offset int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return pageSize, (page - 1) * pageSize
}

// PaginatedResult wrapper hasil query berhalaman.
type PaginatedResult[T any] struct {
	Data       []T   `json:"data"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}

// NewPaginated membuat PaginatedResult.
func NewPaginated[T any](data []T, total int64, page, pageSize int) PaginatedResult[T] {
	totalPages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPages++
	}
	return PaginatedResult[T]{
		Data: data, Total: total,
		Page: page, PageSize: pageSize,
		TotalPages: totalPages,
	}
}
