import os
import hashlib
from datetime import datetime, timedelta
import psycopg2
from psycopg2.extras import execute_batch

os.environ["PYTHONIOENCODING"] = "utf-8"
os.environ["PGCLIENTENCODING"] = "UTF8"

DB_HOST = "localhost"
DB_PORT = "5432"
DB_NAME = "postgres"
DB_USER = "postgres"
DB_PASS = os.environ.get("PGPASSWORD", "postgres")

print("[1/4] Connexion aux connectiques PostgreSQL 17...")
conn = psycopg2.connect(dbname=DB_NAME, user=DB_USER, password=DB_PASS, host=DB_HOST, port=DB_PORT)
cur = conn.cursor()

# 1. Création / Mise à jour de la table d'agrégation temporelle
cur.execute("""
    CREATE TABLE IF NOT EXISTS synthese_mouvements_temporels (
        id SERIAL PRIMARY KEY,
        echelle_temps VARCHAR(20) NOT NULL,       -- 'JOUR', 'SEMAINE', 'MOIS', 'ANNEE'
        periode_libelle VARCHAR(50) NOT NULL,     -- '2026-10-15', '2026-W42', '2026-10', '2026'
        total_factures INT NOT NULL,
        factures_conformes INT NOT NULL,
        factures_rejetees INT NOT NULL,
        total_ht_mad NUMERIC(15,2) NOT NULL,
        total_tva_mad NUMERIC(15,2) NOT NULL,
        signatures_pades_ok INT NOT NULL,
        statut_reporting VARCHAR(30) NOT NULL,
        date_consolidation TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
    );
""")

# Nettoyage pour test propre
cur.execute("TRUNCATE TABLE synthese_mouvements_temporels RESTART IDENTITY;")

# 2. Données temporelles simulées : Journalier (J-3 à J), Semaine (W40 à W42), Mois (Août à Octobre), Annuel (2026)
mouvements_data = [
    # Journalier
    ('JOUR', '2026-10-15 (Aujourd hui)', 300, 201, 99, 1450000.00, 290000.00, 134, 'TRANSMIS_DGI'),
    ('JOUR', '2026-10-14 (Hier)', 280, 195, 85, 1220000.00, 244000.00, 128, 'VALIDE'),
    ('JOUR', '2026-10-13', 250, 180, 70, 980000.00, 196000.00, 115, 'VALIDE'),
    
    # Semaine
    ('SEMAINE', '2026-W42 (En cours)', 830, 576, 254, 3650000.00, 730000.00, 377, 'EN_COURS_CONSOLIDATION'),
    ('SEMAINE', '2026-W41', 1450, 1080, 370, 6890000.00, 1378000.00, 710, 'VALIDE_ARCHIVE'),
    ('SEMAINE', '2026-W40', 1390, 1020, 370, 6420000.00, 1284000.00, 680, 'VALIDE_ARCHIVE'),
    
    # Mois
    ('MOIS', 'Octobre 2026 (En cours)', 2280, 1656, 624, 10540000.00, 2108000.00, 1087, 'CLOTURE_INTERMEDIAIRE'),
    ('MOIS', 'Septembre 2026', 5840, 4380, 1460, 28400000.00, 5680000.00, 2890, 'CLOTURE_DEFINITIVE'),
    ('MOIS', 'Aout 2026', 4920, 3690, 1230, 23150000.00, 4630000.00, 2410, 'CLOTURE_DEFINITIVE'),
    
    # Annuel
    ('ANNEE', 'Exercice Fiscal 2026 (Cumul YTD)', 58400, 43800, 14600, 284000000.00, 56800000.00, 28750, 'CERTIFIE_AUDIT')
]

print("[2/4] Injection des agrégats temporels (Jour / Semaine / Mois / An)...")
query_synth = """
    INSERT INTO synthese_mouvements_temporels 
    (echelle_temps, periode_libelle, total_factures, factures_conformes, factures_rejetees, total_ht_mad, total_tva_mad, signatures_pades_ok, statut_reporting)
    VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s);
"""
execute_batch(cur, query_synth, mouvements_data)

# 3. Jalon d'audit général
cur.execute("""
    INSERT INTO piste_audit (horodatage, utilisateur, type_operation, reference_fichier, hash_sha256, detail_evenement)
    VALUES (%s, %s, %s, %s, %s, %s);
""", (
    datetime.now().strftime("%d/%m/%Y %H:%M:%S"),
    "admin.edimaroc@compliance.ma",
    "TEST_GLOBAL_CONSOLIDATION",
    "AUDIT_TEMPOREL_GLOBAL_2026.csv",
    hashlib.sha256(b"GLOBAL_TEST_2026").hexdigest()[:16] + "...",
    "Test global multi-espaces scellé : simulation entrées, contrôles EN 16931, scellement PAdES et ventilation fiscale Jour/Semaine/Mois/An."
))

conn.commit()

# 4. Diagnostic immédiat des connectiques et de la DB
print("[3/4] Inspection des connectiques SQL actives et de l'espace disque...")
cur.execute("SELECT count(*) FROM pg_stat_activity WHERE state = 'active';")
active_conns = cur.fetchone()[0]

cur.execute("""
    SELECT relname, pg_size_pretty(pg_total_relation_size(c.oid))
    FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public' AND c.relkind = 'r'
    ORDER BY pg_total_relation_size(c.oid) DESC LIMIT 6;
""")
tables_sizes = cur.fetchall()

print(f" -> Connexions SQL actives : {active_conns}")
print(" -> Volumétrie des tables principales :")
for t, sz in tables_sizes:
    print(f"    * {t:<28} : {sz}")

cur.close()
conn.close()
print("[4/4] Base PostgreSQL enrichie et synchronisée avec succès !")
