package main

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type User struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type App struct {
	db    *sql.DB
	token string
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Erreur JSON : %v", err)
	}
}

func (a *App) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")

		if !strings.HasPrefix(header, prefix) {
			writeJSON(w, http.StatusUnauthorized,
				map[string]string{"error": "Authentification requise"})
			return
		}

		token := strings.TrimPrefix(header, prefix)
		if len(token) != len(a.token) ||
			subtle.ConstantTimeCompare([]byte(token), []byte(a.token)) != 1 {
			writeJSON(w, http.StatusUnauthorized,
				map[string]string{"error": "Jeton invalide"})
			return
		}

		next(w, r)
	}
}

func (a *App) listUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := a.db.QueryContext(ctx, `
		SELECT id, username, email, role, created_at
		FROM users ORDER BY id
	`)
	if err != nil {
		log.Printf("Erreur lecture utilisateurs : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Erreur lors de la lecture des utilisateurs"})
		return
	}
	defer rows.Close()

	users := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.CreatedAt); err != nil {
			log.Printf("Erreur lecture ligne : %v", err)
			writeJSON(w, http.StatusInternalServerError,
				map[string]string{"error": "Erreur lors de la lecture des utilisateurs"})
			return
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Erreur parcours lignes : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Erreur lors de la lecture des utilisateurs"})
		return
	}

	writeJSON(w, http.StatusOK, users)
}

func (a *App) createUser(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	var input struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "Corps JSON invalide"})
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	input.Role = strings.ToUpper(strings.TrimSpace(input.Role))

	if input.Username == "" || input.Email == "" {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "username et email sont obligatoires"})
		return
	}

	if input.Role != "ADMIN" && input.Role != "ADHERENT" {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "role doit Ãªtre ADMIN ou ADHERENT"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var u User
	err := a.db.QueryRowContext(ctx, `
		INSERT INTO users (username, email, role)
		VALUES ($1, $2, $3)
		RETURNING id, username, email, role, created_at
	`, input.Username, input.Email, input.Role).Scan(
		&u.ID, &u.Username, &u.Email, &u.Role, &u.CreatedAt,
	)

	if err != nil {
		var stateError interface{ SQLState() string }
		if errors.As(err, &stateError) && stateError.SQLState() == "23505" {
			writeJSON(w, http.StatusConflict,
				map[string]string{"error": "username ou email dÃ©jÃ  utilisÃ©"})
			return
		}

		log.Printf("Erreur crÃ©ation utilisateur : %v", err)
		writeJSON(w, http.StatusInternalServerError,
			map[string]string{"error": "Impossible de crÃ©er l'utilisateur"})
		return
	}

	writeJSON(w, http.StatusCreated, u)
}

func (a *App) metrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := a.db.PingContext(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable,
			map[string]string{"server_status": "DATABASE_UNAVAILABLE"})
		return
	}

	writeJSON(w, http.StatusOK,
		map[string]string{"server_status": "ONLINE"})
}

func initializeDatabase(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id BIGSERIAL PRIMARY KEY,
			username VARCHAR(100) NOT NULL UNIQUE,
			email VARCHAR(254) NOT NULL UNIQUE,
			role VARCHAR(20) NOT NULL
				CHECK (role IN ('ADMIN', 'ADHERENT')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	return err
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL absent : dÃ©marrage refusÃ©")
	}

	token := os.Getenv("ADMIN_API_TOKEN")
	if token == "" {
		log.Fatal("ADMIN_API_TOKEN absent : dÃ©marrage refusÃ©")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("Configuration PostgreSQL impossible : %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(15)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	connected := false
	for attempt := 1; attempt <= 30; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = db.PingContext(ctx)
		cancel()

		if err == nil {
			connected = true
			break
		}

		log.Printf("PostgreSQL indisponible (tentative %d/30)", attempt)
		time.Sleep(2 * time.Second)
	}

	if !connected {
		log.Fatalf("Connexion PostgreSQL impossible : %v", err)
	}

	if err := initializeDatabase(db); err != nil {
		log.Fatalf("Initialisation de la base impossible : %v", err)
	}

	if err := initializeIDPSchema(db); err != nil {
		log.Fatalf("Initialisation IDP impossible : %v", err)
	}
	app := &App{db: db, token: token}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/users", app.authenticate(app.listUsers))
	mux.HandleFunc("POST /api/v1/users", app.authenticate(app.createUser))
	mux.HandleFunc("GET /api/v1/admin/metrics", app.authenticate(app.metrics))
	mux.HandleFunc("POST /api/v1/documents/upload", app.authenticate(app.uploadDocument))
	mux.HandleFunc("POST /api/v1/idp/process", app.authenticate(app.triggerIDP))
	mux.HandleFunc("GET /api/v1/idp/status/{id}", app.authenticate(app.getIDPStatus))
	mux.HandleFunc("GET /api/v1/documents/{id}", app.authenticate(app.getDocument))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Serveur dÃ©marrÃ© sur le port %s", port)
	log.Fatal(server.ListenAndServe())
}
