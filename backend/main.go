package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type User struct {
	Id    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

const (
	apiUsers   = "/api/go/users"
	apiUsersId = "/api/go/users/{id}"
)

// main function
func main() {
	// connect to database
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// create table if it doesn't exist
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS users (id SERIAL PRIMARY KEY, name TEXT, email TEXT)")
	if err != nil {
		log.Fatal(err)
	}

	// create router
	router := mux.NewRouter()
	router.HandleFunc(apiUsers, getUsers(db)).Methods("GET")
	router.HandleFunc(apiUsers, createUser(db)).Methods("POST")
	router.HandleFunc(apiUsersId, getUser(db)).Methods("GET")
	router.HandleFunc(apiUsersId, updateUser(db)).Methods("PUT")
	router.HandleFunc(apiUsersId, deleteUser(db)).Methods("DELETE")

	// wrap the router with CORS and JSON content type middlewares
	enhancedRouter := enableCORS(jsonContentTypeMiddleware(router))
	// start server
	log.Fatal(http.ListenAndServe(":8000", enhancedRouter))
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*") // Allow any origin
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Check if the request is for CORS preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Pass down the request to the next middleware (or final handler)
		next.ServeHTTP(w, r)
	})
}

func jsonContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set JSON Content-Type
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// get all users
func getUsers(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rows, err := db.QueryContext(ctx, "SELECT * FROM users")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		users := []User{} // array of users
		for rows.Next() {
			var u User
			if err := rows.Scan(&u.Id, &u.Name, &u.Email); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			users = append(users, u)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		err = json.NewEncoder(w).Encode(users)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// get user by id
func getUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u User
		err := db.QueryRow("SELECT * FROM users WHERE id = $1", id).Scan(&u.Id, &u.Name, &u.Email)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(u)
	}
}

// create user
func createUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var user User
		json.NewDecoder(r.Body).Decode(&user)

		// Validate input
		if user.Email == "" || !validateEmail(user.Email) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid email"))
			return
		}
		if user.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("name cannot be empty"))
			return
		}

		// Use context for cancellation and timeouts
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Use transaction
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		res, err := tx.ExecContext(ctx, "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id", user.Name, user.Email)
		if err != nil {
			tx.Rollback()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		tx.Commit()

		// Get the inserted user id
		id, err := res.LastInsertId()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		user.Id = int(id)
		json.NewEncoder(w).Encode(user)
	}
}

// update user
func updateUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var u User
		json.NewDecoder(r.Body).Decode(&u)

		if u.Email == "" || !validateEmail(u.Email) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("invalid email"))
			return
		}
		if u.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("name cannot be empty"))
			return
		}

		vars := mux.Vars(r)
		id := vars["id"]

		// Execute the update query
		_, err := db.Exec("UPDATE users SET name = $1, email = $2 WHERE id = $3", u.Name, u.Email, id)
		if err != nil {
			log.Fatal(err)
		}

		// Retrieve the updated user data from the database
		var updatedUser User
		err = db.QueryRow("SELECT id, name, email FROM users WHERE id = $1", id).Scan(&updatedUser.Id, &updatedUser.Name, &updatedUser.Email)
		if err != nil {
			log.Fatal(err)
		}

		// Send the updated user data in the response
		json.NewEncoder(w).Encode(updatedUser)
	}
}

// delete user
func deleteUser(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		id := vars["id"]

		var u User
		err := db.QueryRow("SELECT * FROM users WHERE id = $1", id).Scan(&u.Id, &u.Name, &u.Email)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		} else {
			_, err := db.Exec("DELETE FROM users WHERE id = $1", id)
			if err != nil {
				//todo : fix error handling
				w.WriteHeader(http.StatusNotFound)
				return
			}

			json.NewEncoder(w).Encode("User deleted")
		}
	}
}

func validateEmail(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}
