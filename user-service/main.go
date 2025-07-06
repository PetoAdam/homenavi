package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type User struct {
	ID       string `json:"user_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

var users = []User{
	{ID: uuid.NewString(), Email: "test@example.com", Password: "password", Name: "Test User"},
}

type ValidateRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ValidateResponse struct {
	UserID string `json:"user_id"`
	Valid  bool   `json:"valid"`
}

func main() {
	http.HandleFunc("/validate", handleValidate)
	http.HandleFunc("/user/", handleUserGet)
	http.ListenAndServe(":8001", nil)
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	var req ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	for _, u := range users {
		if u.Email == req.Email && u.Password == req.Password {
			json.NewEncoder(w).Encode(ValidateResponse{UserID: u.ID, Valid: true})
			return
		}
	}
	json.NewEncoder(w).Encode(ValidateResponse{Valid: false})
}

func handleUserGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/user/")
	for _, u := range users {
		if u.ID == id {
			json.NewEncoder(w).Encode(u)
			return
		}
	}
	http.Error(w, "User not found", 404)
}
