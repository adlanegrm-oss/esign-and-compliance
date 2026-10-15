CREATE TABLE IF NOT EXISTS adherent (
    id SERIAL PRIMARY KEY,
    raison_sociale VARCHAR(150) NOT NULL,
    email_contact VARCHAR(255) NOT NULL
);

INSERT INTO adherent (id, raison_sociale, email_contact)
OVERRIDING SYSTEM VALUE
VALUES (1, 'ENTREPRISE IMPORT SARL', 'compta@client-maroc.ma')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS flux_documents (
    id SERIAL PRIMARY KEY,
    adherent_id INT REFERENCES adherent(id) DEFAULT 1,
    numero_document VARCHAR(50) NOT NULL,
    hash_sha256 VARCHAR(64) NOT NULL,
    entite_emettrice VARCHAR(150) NOT NULL,
    entite_receptrice VARCHAR(150) NOT NULL,
    nom_fichier VARCHAR(255) NOT NULL,
    date_traitement VARCHAR(50),
    statut VARCHAR(30) NOT NULL,
    observations TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS piste_audit (
    id SERIAL PRIMARY KEY,
    horodatage VARCHAR(50),
    utilisateur VARCHAR(255) NOT NULL,
    type_operation VARCHAR(50) NOT NULL,
    reference_fichier VARCHAR(255) NOT NULL,
    hash_sha256 VARCHAR(64),
    detail_evenement TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS flux_ereporting (
    id SERIAL PRIMARY KEY,
    type_flux VARCHAR(50) NOT NULL,
    periode_fiscale VARCHAR(50) NOT NULL,
    nombre_operations INT NOT NULL,
    montant_total_ht NUMERIC(15,2) NOT NULL,
    devise VARCHAR(10) NOT NULL,
    statut_transmission VARCHAR(30) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);