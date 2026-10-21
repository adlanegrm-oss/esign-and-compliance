package main

import (
    "crypto/subtle"
    "log"
    "net/http"
    "os"
    "strings"
)

func main() {
    expectedToken := os.Getenv("ADMIN_API_TOKEN")
    if expectedToken == "" {
        log.Fatal("ADMIN_API_TOKEN absent : démarrage refusé")
    }

    mux := http.NewServeMux()

    mux.HandleFunc("/api/v1/admin/metrics", func(w http.ResponseWriter, r *http.Request) {
        authHeader := r.Header.Get("Authorization")

        if !strings.HasPrefix(authHeader, "Bearer ") {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusUnauthorized)
            w.Write([]byte(`{"error":"Authentification requise"}`))
            return
        }

        token := strings.TrimPrefix(authHeader, "Bearer ")
        if token == "" ||
            subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusUnauthorized)
            w.Write([]byte(`{"error":"Jeton invalide"}`))
            return
        }

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"server_status":"ONLINE"}`))
    })

    log.Println("Serveur démarré sur le port 8080")
    log.Fatal(http.ListenAndServe(":8080", mux))
}