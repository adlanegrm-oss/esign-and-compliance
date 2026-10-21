package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNewIDPUUID(t *testing.T) {
	id, err := newIDPUUID()
	if err != nil {
		t.Fatalf("newIDPUUID() erreur : %v", err)
	}
	if !validIDPUUID(id) {
		t.Fatalf("UUID généré invalide : %q", id)
	}
	if id[14] != '4' {
		t.Fatalf("UUID attendu en version 4, reçu : %q", id)
	}
	if !strings.ContainsRune("89ab", rune(id[19])) {
		t.Fatalf("Variante UUID invalide : %q", id)
	}
}

func TestValidIDPUUID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"UUID valide", "010d8141-3f42-4240-9ac6-b386b06414e2", true},
		{"UUID majuscule", "010D8141-3F42-4240-9AC6-B386B06414E2", true},
		{"vide", "", false},
		{"trop court", "010d8141-3f42-4240", false},
		{"sans séparateurs", "010d81413f4242409ac6b386b06414e2", false},
		{"caractère invalide", "zzzzzzzz-3f42-4240-9ac6-b386b06414e2", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := validIDPUUID(tc.id); got != tc.want {
				t.Errorf("validIDPUUID(%q) = %v, attendu %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestSanitizeOriginalFilename(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"chemin Windows", `..\..\secret.pdf`, "secret.pdf"},
		{"chemin Unix", "../../secret.pdf", "secret.pdf"},
		{"nom vide", "", "document"},
		{"espaces seuls", "   ", "document"},
		{"caractère de contrôle", "rap" + "\n" + "port.pdf", "rapport.pdf"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeOriginalFilename(tc.in); got != tc.want {
				t.Errorf("sanitizeOriginalFilename(%q) = %q, attendu %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeOriginalFilenameLength(t *testing.T) {
	got := sanitizeOriginalFilename(strings.Repeat("é", 201))
	if count := utf8.RuneCountInString(got); count != 200 {
		t.Fatalf("longueur = %d runes, attendu 200", count)
	}
	if !utf8.ValidString(got) {
		t.Fatal("le nom de fichier résultant n'est pas de l'UTF-8 valide")
	}
}

func TestAuthenticate(t *testing.T) {
	app := &App{token: "test-secret"}
	handler := app.authenticate(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"sans jeton", "", http.StatusUnauthorized},
		{"jeton incorrect", "Bearer wrong-token", http.StatusUnauthorized},
		{"jeton correct", "Bearer test-secret", http.StatusNoContent},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Errorf("code HTTP = %d, attendu %d", rec.Code, tc.want)
			}
		})
	}
}
