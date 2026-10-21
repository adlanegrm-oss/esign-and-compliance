package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCoverageFixture(t *testing.T, contents string, size int) string {
	t.Helper()

	if len(contents) > size {
		t.Fatalf("fixture content is longer than requested size")
	}

	data := contents + strings.Repeat("X", size-len(contents))
	path := filepath.Join(t.TempDir(), "fixture.pdf")

	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestCheckSingleSignatureCoverage(t *testing.T) {
	tests := []struct {
		name    string
		content string
		size    int
		wantErr bool
	}{
		{
			name:    "complete coverage",
			content: "/ByteRange [0 10 20 10]",
			size:    30,
			wantErr: false,
		},
		{
			name:    "trailing bytes after signed revision",
			content: "/ByteRange [0 10 20 10]",
			size:    31,
			wantErr: true,
		},
		{
			name:    "missing ByteRange",
			content: "not a signed PDF",
			size:    30,
			wantErr: true,
		},
		{
			name:    "malformed ByteRange",
			content: "/ByteRange [zero 10 20 10]",
			size:    40,
			wantErr: true,
		},
		{
			name:    "range exceeds file size",
			content: "/ByteRange [0 999 20 10]",
			size:    40,
			wantErr: true,
		},
		{
			name:    "multiple ByteRange entries",
			content: "/ByteRange [0 10 20 10] /ByteRange [0 10 20 10]",
			size:    80,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeCoverageFixture(t, tt.content, tt.size)
			err := checkSingleSignatureCoverage(path)

			if (err != nil) != tt.wantErr {
				t.Fatalf("checkSingleSignatureCoverage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSignedPDFTampering checks that changing signed bytes invalidates the signature.
func TestSignedPDFTampering(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	if err := runDemo(); err != nil {
		t.Fatalf("generate demo PDF: %v", err)
	}

	data, err := os.ReadFile("invoice-signed.pdf")
	if err != nil {
		t.Fatal(err)
	}

	marker := []byte("Client approval - PoC")
	index := bytes.Index(data, marker)
	if index < 0 {
		t.Fatal("signature reason marker not found in signed PDF")
	}

	// Change a signed byte without changing the file length.
	data[index] = 'X'

	tampered := filepath.Join(tempDir, "invoice-signed-tampered.pdf")
	if err := os.WriteFile(tampered, data, 0600); err != nil {
		t.Fatal(err)
	}

	if err := checkSingleSignatureCoverage(tampered); err != nil {
		t.Fatalf("coverage should remain complete after same-length modification: %v", err)
	}

	if err := verifyPDF(tampered); err == nil {
		t.Fatal("expected cryptographic verification to reject the modified PDF")
	}
}
