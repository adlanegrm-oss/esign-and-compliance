import csv
import sys
import os
import glob
import psycopg2
from psycopg2.extras import execute_batch

os.environ["PYTHONIOENCODING"] = "utf-8"
os.environ["PGCLIENTENCODING"] = "UTF8"

DB_HOST = "localhost"
DB_PORT = "5432"
DB_NAME = "postgres"
DB_USER = "postgres"

def trouver_fichier(motif):
    # Cherche d'abord dans le workspace, puis dans le dossier Téléchargements de l'utilisateur
    dossiers = [
        "D:\\EDIMAROC_WORKSPACE",
        os.path.expanduser("~\\Downloads")
    ]
    for d in dossiers:
        fichiers = glob.glob(os.path.join(d, motif))
        if fichiers:
            # Prendre le plus récent
            fichiers.sort(key=os.path.getmtime, reverse=True)
            return fichiers[0]
    return None

def executer_injection():
    print(f"Connexion a PostgreSQL ({DB_USER}@{DB_HOST}:{DB_PORT}/{DB_NAME})...")
    password = os.environ.get("PGPASSWORD", "postgres")

    conn = psycopg2.connect(
        dbname=DB_NAME,
        user=DB_USER,
        password=password,
        host=DB_HOST,
        port=DB_PORT
    )
    cur = conn.cursor()
    print("Connexion PostgreSQL etablie avec succes !")

    # 1. Vérification des tables
    if os.path.exists("schema.sql"):
        with open("schema.sql", "r", encoding="utf-8-sig") as f:
            cur.execute(f.read().strip())
        conn.commit()

    # 2. Injection du Rapport de Conformité (100 factures)
    f_compliance = trouver_fichier("*Rapport_Conformite*Compliance*.csv")
    if f_compliance:
        print(f"Fichier compliance detecte : {f_compliance}")
        docs = []
        with open(f_compliance, "r", encoding="utf-8-sig") as f:
            reader = csv.reader(f, delimiter=";")
            header = next(reader, None)
            for row in reader:
                if len(row) >= 8:
                    docs.append((row[0], row[1], row[2], row[3], row[4], row[5], row[6], row[7]))

        if docs:
            cur.execute("TRUNCATE TABLE flux_documents RESTART IDENTITY;")
            query_doc = """
                INSERT INTO flux_documents 
                (numero_document, hash_sha256, entite_emettrice, entite_receptrice, nom_fichier, date_traitement, statut, observations)
                VALUES (%s, %s, %s, %s, %s, %s, %s, %s);
            """
            execute_batch(cur, query_doc, docs)
            print(f"-> {len(docs)} factures injectees dans 'flux_documents'.")
    else:
        print("[!] Aucun fichier de conformite trouve.")

    # 3. Injection de la Piste d'Audit
    f_audit = trouver_fichier("*Piste_Audit*.csv")
    if f_audit:
        print(f"Fichier piste d'audit detecte : {f_audit}")
        audits = []
        with open(f_audit, "r", encoding="utf-8-sig") as f:
            reader = csv.reader(f, delimiter=";")
            header = next(reader, None)
            for row in reader:
                if len(row) >= 6:
                    audits.append((row[0], row[1], row[2], row[3], row[4], row[5]))

        if audits:
            cur.execute("TRUNCATE TABLE piste_audit RESTART IDENTITY;")
            query_audit = """
                INSERT INTO piste_audit 
                (horodatage, utilisateur, type_operation, reference_fichier, hash_sha256, detail_evenement)
                VALUES (%s, %s, %s, %s, %s, %s);
            """
            execute_batch(cur, query_audit, audits)
            print(f"-> {len(audits)} evenements d'audit injectes dans 'piste_audit'.")

    # 4. Injection de l'E-Reporting
    f_ereport = trouver_fichier("*E_Reporting*.csv")
    if f_ereport:
        print(f"Fichier e-reporting detecte : {f_ereport}")
        ereports = []
        with open(f_ereport, "r", encoding="utf-8-sig") as f:
            reader = csv.reader(f, delimiter=";")
            header = next(reader, None)
            for row in reader:
                if len(row) >= 6:
                    try:
                        nb_ops = int(row[2])
                        mt_ht = float(row[3])
                        ereports.append((row[0], row[1], nb_ops, mt_ht, row[4], row[5]))
                    except ValueError:
                        continue

        if ereports:
            cur.execute("TRUNCATE TABLE flux_ereporting RESTART IDENTITY;")
            query_ereport = """
                INSERT INTO flux_ereporting 
                (type_flux, periode_fiscale, nombre_operations, montant_total_ht, devise, statut_transmission)
                VALUES (%s, %s, %s, %s, %s, %s);
            """
            execute_batch(cur, query_ereport, ereports)
            print(f"-> {len(ereports)} enregistrements injectes dans 'flux_ereporting'.")

    conn.commit()

    # Contrôle direct des comptages en base
    print("\n=== VERIFICATION DES DONNEES EN BASE POSTGRESQL ===")
    cur.execute("SELECT statut, COUNT(*) FROM flux_documents GROUP BY statut;")
    for r in cur.fetchall():
        print(f"Table flux_documents  -> Statut: {r[0]:<15} Total: {r[1]}")

    cur.execute("SELECT COUNT(*) FROM piste_audit;")
    print(f"Table piste_audit     -> Total enregistrements: {cur.fetchone()[0]}")

    cur.execute("SELECT COUNT(*) FROM flux_ereporting;")
    print(f"Table flux_ereporting -> Total flux: {cur.fetchone()[0]}")

    cur.close()
    conn.close()
    print("\nInjection PostgreSQL terminee avec succes !")

if __name__ == "__main__":
    executer_injection()
