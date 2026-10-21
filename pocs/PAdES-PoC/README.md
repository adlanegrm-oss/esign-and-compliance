# PAdES PoC starter (isolated from the SaaS repository)

This is a **development proof of concept**, not production signing software. It does not implement user authentication, consent capture, tenant isolation, encrypted key storage, certificate revocation workflows, trusted identity, legal qualification, or an API.

## Requirements

- Go installed (the module uses `github.com/digitorus/pdfsign` v0.9.0).
- Internet access on first run so Go can download modules.

## Run on Windows PowerShell

1. Extract this ZIP into a separate folder, **not inside your SaaS repository**.
2. Open PowerShell in that folder.
3. Run:

   ```powershell
   go mod tidy
   go run . demo
   ```

The demo creates `dev-cert.pem`, `dev-key.pem`, `invoice-sample.pdf`, and `invoice-signed.pdf`, then asks the library to verify the signature. The certificate and private key are development-only artifacts; do not upload or reuse them. Files are written into the current directory.

To verify again:

```powershell
go run . verify .\invoice-signed.pdf
```

To sign a different PDF using a PEM certificate and RSA private key:

```powershell
go run . sign .\input.pdf .\signed.pdf .\cert.pem .\private-key.pem
```

## Independent verification (recommended)

Use a separate PDF validator/reader in addition to this program. For example, install Poppler for Windows and run `pdfsig .\invoice-signed.pdf`, or open the PDF in Adobe Acrobat Reader and inspect the signature panel. A self-signed test certificate will normally show as untrusted; that is expected. Check both the cryptographic integrity result and the trust/identity status separately.

## Tamper test

1. Keep an untouched copy of `invoice-signed.pdf`.
2. Open a copy in a PDF editor and change the document after signing, then save it as a new file.
3. Run `go run . verify .\tampered.pdf` and inspect the independent validator too. Some editors may invalidate or add incremental revisions rather than simply breaking the file; record exactly what each validator reports.

## Important boundaries

- The PoC deliberately does not call a timestamp authority (TSA), so no document is sent to an external TSA by this starter.
- The demo verifier explicitly trusts a self-signed development certificate to test the cryptographic path. That is **not** a production trust policy.
- PAdES format alone does not make a signature advanced or qualified. Do not advertise this as a qualified signature or claim guaranteed legal effect.
- Do not import real client certificates or real invoices into this PoC.
- Before production, separately design authorization/consent, tenant isolation, encrypted key vault, certificate lifecycle and revocation, audit trail, idempotency, recovery, key rotation, secure deployment, and legal review for Morocco and the EU.

## Dependency reference

`github.com/digitorus/pdfsign` (BSD-2-Clause): https://github.com/digitorus/pdfsign

## Correction for pdfsign v0.9.0
The root import `github.com/digitorus/pdfsign` is a command package in v0.9.0 and cannot be imported by an application. This corrected starter uses the supported `github.com/digitorus/pdfsign/sign` and `github.com/digitorus/pdfsign/verify` subpackages. It is still a development PoC, not production-ready code or a legal qualification claim.
