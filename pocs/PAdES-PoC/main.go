// pades-poc-starter is an isolated proof of concept. It is not production code.
package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/digitorus/pdfsign/sign"
	pdfverify "github.com/digitorus/pdfsign/verify"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "demo":
		err = runDemo()
	case "sign":
		if len(os.Args) != 6 {
			usage()
			os.Exit(2)
		}
		err = signPDF(os.Args[2], os.Args[3], os.Args[4], os.Args[5])
	case "verify":
		if len(os.Args) != 3 {
			usage()
			os.Exit(2)
		}
		err = verifyPDF(os.Args[2])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`Commands:
  go run . demo
  go run . sign <input.pdf> <output.pdf> <cert.pem> <private-key.pem>
  go run . verify <signed.pdf>

The demo creates a self-signed DEVELOPMENT certificate, a synthetic invoice PDF,
then signs and verifies it. Self-signed trust in this demo is not production trust.`)
}

func runDemo() error {
	if err := writeDevCertificate("dev-cert.pem", "dev-key.pem"); err != nil {
		return err
	}
	if err := writeSamplePDF("invoice-sample.pdf"); err != nil {
		return err
	}
	if err := signPDF("invoice-sample.pdf", "invoice-signed.pdf", "dev-cert.pem", "dev-key.pem"); err != nil {
		return err
	}
	return verifyPDF("invoice-signed.pdf")
}

func writeDevCertificate(certPath, keyPath string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: "PAdES PoC Development Only", Organization: []string{"NOT FOR PRODUCTION"}},
		NotBefore:    now.Add(-5 * time.Minute), NotAfter: now.Add(30 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certFile, err := os.OpenFile(certPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		certFile.Close()
		return err
	}
	if err = certFile.Close(); err != nil {
		return err
	}
	keyFile, err := os.OpenFile(keyPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		keyFile.Close()
		return err
	}
	return keyFile.Close()
}

func writeSamplePDF(path string) error {
	// A minimal one-page PDF containing a synthetic invoice summary.
	stream := "BT /F1 18 Tf 72 740 Td (SAMPLE INVOICE - NOT A REAL TAX DOCUMENT) Tj 0 -32 Td /F1 12 Tf (Invoice: POC-0001) Tj 0 -20 Td (Customer: Example Client) Tj 0 -20 Td (Amount: EUR 100.00) Tj ET"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return os.WriteFile(path, []byte(b.String()), 0600)
}

func signPDF(input, output, certPath, keyPath string) error {
	if strings.TrimSpace(input) == "" || strings.TrimSpace(output) == "" {
		return errors.New("input/output path required")
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return errors.New("certificate must be PEM CERTIFICATE")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return errors.New("private key must be PEM")
	}
	var signer *rsa.PrivateKey
	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		signer, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err == nil {
			var ok bool
			signer, ok = parsed.(*rsa.PrivateKey)
			if !ok {
				return errors.New("only RSA keys are supported by this PoC")
			}
		}
	default:
		return fmt.Errorf("unsupported private-key PEM type %q", keyBlock.Type)
	}
	if err != nil {
		return err
	}
	certPub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("certificate public key is not RSA")
	}
	if signer.PublicKey.N.Cmp(certPub.N) != 0 || signer.PublicKey.E != certPub.E {
		return errors.New("certificate/private-key mismatch")
	}

	// The v0.9.0 root package is a CLI program, not an importable Go library.
	// Use the supported sign subpackage API. No TSA is configured in this PoC.
	sd := sign.SignData{
		Signature: sign.SignDataSignature{
			CertType: sign.ApprovalSignature,
			Info: sign.SignDataSignatureInfo{
				Name:     "PAdES PoC Development Only",
				Location: "Local test",
				Reason:   "Client approval - PoC",
				Date:     time.Now(),
			},
		},
		Signer:          signer,
		DigestAlgorithm: crypto.SHA256,
		Certificate:     cert,
	}
	return sign.SignFile(input, output, sd)
}

func checkSingleSignatureCoverage(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	re := regexp.MustCompile(`/ByteRange\s*\[\s*(\d+)\s+(\d+)\s+(\d+)\s+(\d+)\s*\]`)
	matches := re.FindAllSubmatch(data, -1)

	if len(matches) != 1 {
		return fmt.Errorf("PoC coverage check requires exactly one ByteRange; found %d", len(matches))
	}

	var v [4]int64
	for i := 0; i < 4; i++ {
		n, err := strconv.ParseInt(string(matches[0][i+1]), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid ByteRange value: %w", err)
		}
		v[i] = n
	}

	start1, length1, start2, length2 := v[0], v[1], v[2], v[3]
	size := int64(len(data))

	if start1 != 0 || length1 < 0 || start2 < 0 || length2 < 0 ||
		length1 > size || start2 > size || length2 > size-start2 ||
		start2 < start1+length1 {
		return errors.New("invalid PDF signature ByteRange")
	}

	if start2+length2 != size {
		return fmt.Errorf(
			"signature does not cover the complete current PDF: signed revision ends at byte %d, file size is %d",
			start2+length2, size,
		)
	}

	return nil
}
func verifyPDF(path string) error {
	if err := checkSingleSignatureCoverage(path); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	options := pdfverify.DefaultVerifyOptions()
	options.AllowUntrustedRoots = true // development certificate only; never production trust
	options.AllowedEKUs = []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection}
	resp, err := pdfverify.VerifyFileWithOptions(f, options)
	if err != nil {
		return err
	}
	if resp.Error != "" {
		return fmt.Errorf("signature verification failed: %s", resp.Error)
	}
	if len(resp.Signers) == 0 {
		return errors.New("no PDF signatures were found")
	}
	for _, sig := range resp.Signers {
		if !sig.ValidSignature {
			return errors.New("signature is not cryptographically valid according to the PoC verifier")
		}
		fmt.Printf("Cryptographic signature valid: %s | Reason: %s\n", sig.Name, sig.Reason)
	}
	fmt.Println("WARNING: this demo explicitly allows a self-signed development certificate; this does not establish identity, certificate trust, legal qualification, or production validity.")
	return nil
}
