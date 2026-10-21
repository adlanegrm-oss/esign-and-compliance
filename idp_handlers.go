package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const maxIDPUpload int64 = 20 << 20

var idpUUIDPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

func newIDPUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16],
	), nil
}

func validIDPUUID(id string) bool {
	return idpUUIDPattern.MatchString(id)
}

func idpUploadDir() string {
	dir := strings.TrimSpace(os.Getenv("UPLOAD_DIR"))
	if dir == "" {
		return "./uploads"
	}
	return dir
}

func sanitizeOriginalFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)

	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		return "document"
	}

	runes := []rune(name)
	if len(runes) > 200 {
		name = string(runes[:200])
	}

	return name
}

// initializeIDPSchema crée les tables IDP dans le schéma par défaut
// du compte PostgreSQL utilisé par le backend.
func initializeIDPSchema(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS documents (
id UUID PRIMARY KEY,
original_filename TEXT NOT NULL,
mime_type TEXT NOT NULL,
storage_path TEXT NOT NULL UNIQUE,
file_size BIGINT NOT NULL CHECK (file_size > 0),
sha256 CHAR(64) NOT NULL,
status TEXT NOT NULL DEFAULT 'received'
CHECK (status IN (
'received', 'queued', 'processing', 'extracted',
'validated', 'signed', 'rejected', 'failed'
)),
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`,
		`CREATE TABLE IF NOT EXISTS document_extractions (
id UUID PRIMARY KEY,
document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
document_type TEXT,
extracted_data JSONB NOT NULL,
confidence_score DOUBLE PRECISION
CHECK (confidence_score >= 0.0 AND confidence_score <= 1.0),
structured_file_path TEXT,
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`,
		`CREATE TABLE IF NOT EXISTS idp_jobs (
id UUID PRIMARY KEY,
document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
status TEXT NOT NULL DEFAULT 'pending'
CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
error_message TEXT,
created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
started_at TIMESTAMPTZ,
finished_at TIMESTAMPTZ
)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_status
ON documents(status)`,
		`CREATE INDEX IF NOT EXISTS idx_extractions_doc_created
ON document_extractions(document_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_idp_jobs_doc_created
ON idp_jobs(document_id, created_at DESC)`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("initialisation du schéma IDP : %w", err)
		}
	}

	return nil
}

// uploadDocument accepte uniquement des formats détectés par leur contenu.
// Le nom envoyé par l'utilisateur n'est jamais utilisé comme chemin de stockage.
func (a *App) uploadDocument(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxIDPUpload+(1<<20))

	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge,
				map[string]string{"error": "Fichier trop volumineux"})
		} else {
			writeJSON(w, http.StatusBadRequest,
				map[string]string{"error": "Requête multipart invalide"})
		}
		return
	}

	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "Champ fichier manquant"})
		return
	}
	defer file.Close()

	if header.Size <= 0 {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "Fichier vide"})
		return
	}
	if header.Size > maxIDPUpload {
		writeJSON(w, http.StatusRequestEntityTooLarge,
			map[string]string{"error": "Taille maximale de 20 Mio dépassée"})
		return
	}

	sample := make([]byte, 512)
	n, readErr := file.Read(sample)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "Lecture du fichier impossible"})
		return
	}

	mimeType := http.DetectContentType(sample[:n])
	extensions := map[string]string{
		"application/pdf": ".pdf",
		"image/jpeg":      ".jpg",
		"image/png":       ".png",
		"image/gif":       ".gif",
		"image/webp":      ".webp",
	}
	extension, allowed := extensions[mimeType]
	if !allowed {
		writeJSON(w, http.StatusUnsupportedMediaType,
			map[string]string{"error": "Format accepté : PDF, JPEG, PNG, GIF ou WebP"})
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Réinitialisation du fichier impossible"})
		return
	}

	docID, err := newIDPUUID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Génération de l'identifiant impossible"})
		return
	}

	dir := idpUploadDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Printf("Création du répertoire IDP impossible : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Stockage indisponible"})
		return
	}

	storageName := docID + extension
	destPath := filepath.Join(dir, storageName)

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		log.Printf("Création du fichier IDP impossible : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Écriture du fichier impossible"})
		return
	}

	hasher := sha256.New()
	written, copyErr := io.Copy(
		io.MultiWriter(dst, hasher),
		io.LimitReader(file, maxIDPUpload+1),
	)
	closeErr := dst.Close()

	if copyErr != nil || closeErr != nil || written <= 0 {
		_ = os.Remove(destPath)
		log.Printf("Écriture du fichier IDP incomplète : copy=%v close=%v", copyErr, closeErr)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Écriture du fichier impossible"})
		return
	}

	if written > maxIDPUpload {
		_ = os.Remove(destPath)
		writeJSON(w, http.StatusRequestEntityTooLarge,
			map[string]string{"error": "Taille maximale de 20 Mio dépassée"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err = a.db.ExecContext(ctx, `
INSERT INTO documents
(id, original_filename, mime_type, storage_path, file_size, sha256, status)
VALUES ($1, $2, $3, $4, $5, $6, 'received')
`,
		docID,
		sanitizeOriginalFilename(header.Filename),
		mimeType,
		storageName,
		written,
		hex.EncodeToString(hasher.Sum(nil)),
	)
	if err != nil {
		_ = os.Remove(destPath)
		log.Printf("Insertion du document IDP impossible : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Enregistrement du document impossible"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"document_id": docID,
		"status":      "received",
		"filename":    sanitizeOriginalFilename(header.Filename),
		"mime_type":   mimeType,
		"size_bytes":  written,
	})
}

// triggerIDP crée un job en attente. Le worker OCR/RAD/LAD sera raccordé
// dans une étape suivante ; aucun traitement n'est simulé ici.
func (a *App) triggerIDP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)

	var req struct {
		DocumentID string `json:"document_id"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil || !validIDPUUID(req.DocumentID) {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "document_id UUID requis"})
		return
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "Un seul objet JSON est autorisé"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Création du job impossible"})
		return
	}
	defer tx.Rollback()

	var docStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM documents WHERE id = $1 FOR UPDATE`,
		req.DocumentID,
	).Scan(&docStatus)

	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound,
			map[string]string{"error": "Document introuvable"})
		return
	}
	if err != nil {
		log.Printf("Lecture du document IDP impossible : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Lecture du document impossible"})
		return
	}

	if docStatus != "received" && docStatus != "failed" {
		writeJSON(w, http.StatusConflict,
			map[string]string{"error": "Document non disponible pour un nouveau traitement"})
		return
	}

	jobID, err := newIDPUUID()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Génération du job impossible"})
		return
	}

	_, err = tx.ExecContext(ctx, `
INSERT INTO idp_jobs (id, document_id, status)
VALUES ($1, $2, 'pending')
`, jobID, req.DocumentID)
	if err != nil {
		log.Printf("Insertion du job IDP impossible : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Création du job impossible"})
		return
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE documents SET status = 'queued' WHERE id = $1`,
		req.DocumentID,
	)
	if err != nil {
		log.Printf("Mise à jour du statut IDP impossible : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Mise à jour du document impossible"})
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("Validation du job IDP impossible : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Enregistrement du job impossible"})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":      jobID,
		"document_id": req.DocumentID,
		"status":      "pending",
	})
}

func (a *App) getIDPStatus(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("id")
	if !validIDPUUID(docID) {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "Identifiant de document invalide"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var documentStatus string
	var jobStatus, jobError, documentType sql.NullString
	var extractedData []byte
	var confidence sql.NullFloat64

	err := a.db.QueryRowContext(ctx, `
SELECT d.status, j.status, j.error_message,
       e.document_type, e.extracted_data, e.confidence_score
FROM documents d
LEFT JOIN LATERAL (
SELECT status, error_message
FROM idp_jobs
WHERE document_id = d.id
ORDER BY created_at DESC
LIMIT 1
) j ON TRUE
LEFT JOIN LATERAL (
SELECT document_type, extracted_data, confidence_score
FROM document_extractions
WHERE document_id = d.id
ORDER BY created_at DESC
LIMIT 1
) e ON TRUE
WHERE d.id = $1
`, docID).Scan(
		&documentStatus, &jobStatus, &jobError,
		&documentType, &extractedData, &confidence,
	)

	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound,
			map[string]string{"error": "Document introuvable"})
		return
	}
	if err != nil {
		log.Printf("Lecture du statut IDP impossible : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Lecture du statut impossible"})
		return
	}

	result := map[string]any{
		"document_id":     docID,
		"document_status": documentStatus,
		"job_status":      "not_started",
	}

	if jobStatus.Valid {
		result["job_status"] = jobStatus.String
	}
	if jobError.Valid {
		result["error"] = jobError.String
	}
	if documentType.Valid {
		result["document_type"] = documentType.String
	}
	if confidence.Valid {
		result["confidence_score"] = confidence.Float64
	}
	if len(extractedData) > 0 {
		var parsed any
		if err := json.Unmarshal(extractedData, &parsed); err != nil {
			log.Printf("JSON d'extraction IDP invalide pour %s", docID)
			writeJSON(w, http.StatusInternalServerError,
				map[string]string{"error": "Résultat d'extraction illisible"})
			return
		}
		result["extracted_data"] = parsed
	}

	writeJSON(w, http.StatusOK, result)
}

func (a *App) getDocument(w http.ResponseWriter, r *http.Request) {
	docID := r.PathValue("id")
	if !validIDPUUID(docID) {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "Identifiant de document invalide"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var filename, mimeType, status, sha256sum string
	var size int64
	var createdAt time.Time

	err := a.db.QueryRowContext(ctx, `
SELECT original_filename, mime_type, status, file_size, sha256, created_at
FROM documents
WHERE id = $1
`, docID).Scan(
		&filename, &mimeType, &status, &size, &sha256sum, &createdAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound,
			map[string]string{"error": "Document introuvable"})
		return
	}
	if err != nil {
		log.Printf("Lecture des métadonnées IDP impossible : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Lecture du document impossible"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"document_id":       docID,
		"original_filename": filename,
		"mime_type":         mimeType,
		"size_bytes":        size,
		"sha256":            strings.TrimSpace(sha256sum),
		"status":            status,
		"created_at":        createdAt,
	})
}
