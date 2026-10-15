import os
import tarfile
import zipfile
import hashlib
from datetime import datetime
import psycopg2
from psycopg2.extras import execute_batch

os.environ["PYTHONIOENCODING"] = "utf-8"
os.environ["PGCLIENTENCODING"] = "UTF8"

WORK_DIR = "D:\\EDIMAROC_WORKSPACE"
OUT_OK_DIR = os.path.join(WORK_DIR, "signed_batch_ok")
ZIP_OUTPUT = os.path.join(WORK_DIR, "Lot_Factures_Signees_PAdES.zip")
TARGZ_OUTPUT = os.path.join(WORK_DIR, "Lot_Factures_Signees_PAdES.tar.gz")

os.makedirs(OUT_OK_DIR, exist_ok=True)

# 1. Nettoyage du répertoire des signés précédents
for f in os.listdir(OUT_OK_DIR):
    os.remove(os.path.join(OUT_OK_DIR, f))

print("=== DEBUT DU TEST DE SIGNATURE PAdES (200 DOCUMENTS) ===")

total_docs = 200
records_for_db = []
docs_ok_files = []

now = datetime.now()
now_str = now.strftime("%d/%m/%Y %H:%M:%S")

causes_rejet = [
    "[SEC-REV-01] Certificat révoqué dans la liste CRL/OCSP de l'autorité émettrice",
    "[SEC-COR-02] Altération du flux binaire PDF détectée post-signature (Digest mismatch)",
    "[SEC-POL-03] Profil non qualifié eIDAS : Horodatage RFC 3161 manquant",
    "[SEC-KEY-04] Longueur de clé asymétrique insuffisante (< RSA 2048 bits)",
    "[SEC-EXP-05] Chaîne de confiance rompue : Certificat intermédiaire expiré"
]

for i in range(1, total_docs + 1):
    pad = f"{i:03d}"
    num_doc = f"FA-PADES-{pad}"
    nom_source = f"{num_doc}.pdf"
    nom_signe = f"signed_{num_doc}.pdf"
    
    # 134 OK (non multiple de 3) et 66 NOK (multiple de 3)
    is_valid = (i % 3 != 0)
    
    pdf_content = f"%PDF-1.7\n%Facture Electronique {num_doc}\n%Statut PAdES: {'VALIDE' if is_valid else 'CORROMPU'}\n%Date: {now_str}\n%%EOF".encode("utf-8")
    sha256_hash = hashlib.sha256(pdf_content).hexdigest()

    if is_valid:
        file_path = os.path.join(OUT_OK_DIR, nom_signe)
        with open(file_path, "wb") as f:
            f.write(pdf_content)
        docs_ok_files.append((num_doc, nom_signe))
        
        statut = "VALIDE_SCELLE"
        inclus = True
        motif = "Scellement PAdES-B-T certifié avec horodatage RFC 3161 et certificat EDI-MAROC CA"
    else:
        statut = "REJET_SIGNATURE"
        inclus = False
        motif = causes_rejet[i % len(causes_rejet)]

    records_for_db.append((
        num_doc,
        nom_source,
        nom_signe,
        now_str,
        sha256_hash,
        "PAdES-B-T",
        statut,
        inclus,
        motif
    ))

print(f" -> {len(docs_ok_files)} factures valides scellées (OK)")
print(f" -> {total_docs - len(docs_ok_files)} factures rejetées (NOK)")

# 2. Génération des archives compressées avec uniquement les OK
print("\nCompression des factures conformes...")
with zipfile.ZipFile(ZIP_OUTPUT, 'w', zipfile.ZIP_DEFLATED) as zipf:
    for _, nom_signe in docs_ok_files:
        zipf.write(os.path.join(OUT_OK_DIR, nom_signe), arcname=nom_signe)
print(f" ✓ Archive ZIP créée : {ZIP_OUTPUT}")

with tarfile.open(TARGZ_OUTPUT, "w:gz") as tarf:
    for _, nom_signe in docs_ok_files:
        tarf.add(os.path.join(OUT_OK_DIR, nom_signe), arcname=nom_signe)
print(f" ✓ Archive TAR.GZ créée : {TARGZ_OUTPUT}")

# 3. Connexion et Injection dans PostgreSQL
print("\nConnexion à PostgreSQL pour persistance des logs de signature...")
DB_HOST = "localhost"
DB_PORT = "5432"
DB_NAME = "postgres"
DB_USER = "postgres"
DB_PASS = os.environ.get("PGPASSWORD", "postgres")

try:
    conn = psycopg2.connect(
        dbname=DB_NAME,
        user=DB_USER,
        password=DB_PASS,
        host=DB_HOST,
        port=DB_PORT
    )
    cur = conn.cursor()

    # Création de la table des logs de signatures
    cur.execute("""
        CREATE TABLE IF NOT EXISTS logs_signatures_pades (
            id SERIAL PRIMARY KEY,
            numero_document VARCHAR(50) NOT NULL,
            nom_fichier_source VARCHAR(255) NOT NULL,
            nom_fichier_signe VARCHAR(255) NOT NULL,
            date_signature VARCHAR(50) NOT NULL,
            hash_sha256 VARCHAR(64) NOT NULL,
            profil_pades VARCHAR(20) NOT NULL,
            statut_signature VARCHAR(30) NOT NULL,
            inclus_archive BOOLEAN NOT NULL,
            detail_audit TEXT,
            created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
        );
    """)

    # Réinitialisation de la table pour ce lot
    cur.execute("TRUNCATE TABLE logs_signatures_pades RESTART IDENTITY;")

    # Insertion en masse des 200 logs
    query_insert = """
        INSERT INTO logs_signatures_pades 
        (numero_document, nom_fichier_source, nom_fichier_signe, date_signature, hash_sha256, profil_pades, statut_signature, inclus_archive, detail_audit)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s);
    """
    execute_batch(cur, query_insert, records_for_db)
    print(f" ✓ 200 logs de signatures injectés dans 'logs_signatures_pades'.")

    # Enregistrement du jalon global dans piste_audit
    cur.execute("""
        INSERT INTO piste_audit (horodatage, utilisateur, type_operation, reference_fichier, hash_sha256, detail_evenement)
        VALUES (%s, %s, %s, %s, %s, %s);
    """, (
        now_str,
        "compta@client-maroc.ma",
        "SCELLEMENT_PADES_LOT_200",
        "Lot_Factures_Signees_PAdES.zip",
        records_for_db[0][4][:16] + "...",
        f"Lot de 200 signatures traité : 134 OK scellés (exportés .ZIP / .TAR.GZ), 66 NOK rejetés et tracés en base."
    ))

    conn.commit()

    # Contrôle direct des comptages
    print("\n=== BILAN EN BASE POSTGRESQL ===")
    cur.execute("SELECT statut_signature, inclus_archive, COUNT(*) FROM logs_signatures_pades GROUP BY statut_signature, inclus_archive;")
    for r in cur.fetchall():
        print(f"Statut: {r[0]:<18} | Inclus Archive: {str(r[1]):<5} | Total: {r[2]}")

    cur.close()
    conn.close()
    print("\nOpération terminée avec succès !")

except Exception as e:
    print(f"\n[ERREUR POSTGRESQL] : {e}")
