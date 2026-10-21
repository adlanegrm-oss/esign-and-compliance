from pathlib import Path
from pyhanko.pdf_utils.reader import PdfFileReader
from pyhanko.sign.validation import validate_pdf_signature
from pyhanko_certvalidator import ValidationContext

files = [
    Path("invoice-signed.pdf"),
    Path("invoice-signed-tampered.pdf"),
]

for path in files:
    print(f"\n=== {path} ===")

    try:
        with path.open("rb") as stream:
            reader = PdfFileReader(stream)
            signatures = reader.embedded_signatures

            if not signatures:
                print("Aucune signature trouvee.")
                continue

            for sig in signatures:
                context = ValidationContext(allow_fetching=False)
                status = validate_pdf_signature(
                    sig,
                    signer_validation_context=context,
                )

                print("Integrite :", status.intact)
                print("Validite cryptographique :", status.valid)
                print("Certificat de confiance :", status.trusted)
                if status.intact and status.valid:
                    print("VERDICT : signature valide, intégrité préservée.")
                else:
                    print("VERDICT : document altéré ou signature non valide.")

    except Exception as exc:
        print("Erreur de validation :", repr(exc))

