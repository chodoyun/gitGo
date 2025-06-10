package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

// DB 연결 변수(MSSQL)
var db *sql.DB

// 설정 구조체
type Config struct {
	DBServer   string
	DBUser     string
	DBPassword string
	DBPort     string
	DBName     string
	APIKey     string
	Port       string
}

// 환경변수 로드 함수
func loadConfig() *Config {
	// .env 파일이 존재하면 로드 (개발환경용)
	if _, err := os.Stat(".env"); err == nil {
		err := godotenv.Load()
		if err != nil {
			log.Println("Warning: .env 파일 로드 실패, 시스템 환경변수 사용")
		}
	}

	config := &Config{
		DBServer:   getEnv("DB_SERVER", ""),
		DBUser:     getEnv("DB_USER", ""),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBPort:     getEnv("DB_PORT", "1433"),
		DBName:     getEnv("DB_NAME", ""),
		APIKey:     getEnv("API_KEY", ""),
		Port:       getEnv("PORT", "8000"),
	}

	// 필수 환경변수 검증
	if config.DBServer == "" || config.DBUser == "" || config.DBPassword == "" ||
		config.DBName == "" || config.APIKey == "" {
		log.Fatal("필수 환경변수가 설정되지 않았습니다. DB_SERVER, DB_USER, DB_PASSWORD, DB_NAME, API_KEY를 확인하세요.")
	}

	return config
}

// 환경변수 값 가져오기 (기본값 포함)
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// API 키 인증 미들웨어
func authMiddleware(apiKey string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			requestAPIKey := r.Header.Get("X-API-Key")
			if requestAPIKey == "" {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "API 키가 필요합니다"})
				return
			}

			if requestAPIKey != apiKey {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "유효하지 않은 API 키입니다"})
				return
			}

			next.ServeHTTP(w, r)
		}
	}
}

// DB 연결 함수
func connectDB(config *Config) {
	// MSSQL 연결 문자열
	connString := fmt.Sprintf("server=%s;user id=%s;password=%s;port=%s;database=%s",
		config.DBServer, config.DBUser, config.DBPassword, config.DBPort, config.DBName)

	// DB 연결
	var err error
	db, err = sql.Open("mssql", connString)
	if err != nil {
		log.Fatal("DB 연결 실패:", err)
	}

	// 연결 테스트
	err = db.Ping()
	if err != nil {
		log.Fatal("DB 연결 테스트 실패:", err)
	}

	log.Println("MSSQL DB 연결 성공!")
}

type Book struct {
	ID      string `json:"id,omitempty"`
	Title   string `json:"title,omitempty"`
	Author  string `json:"author,omitempty"`
	Year    int    `json:"year,omitempty"`
	Regdate string `json:"regdate,omitempty"`
}

var books []Book

// 헬스체크 엔드포인트 추가
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// 모든 책 정보 조회
func GetBooks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(books)
}

// 특정 ID의 책 정보 조회
func GetBook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := mux.Vars(r)
	id := params["id"]

	for _, book := range books {
		if book.ID == id {
			json.NewEncoder(w).Encode(book)
			return
		}
	}

	// 책을 찾지 못한 경우
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{"error": "책을 찾을 수 없습니다"})
}

// 새로운 책 추가
func CreateBook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var book Book
	err := json.NewDecoder(r.Body).Decode(&book)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "잘못된 요청 형식입니다"})
		return
	}

	// DB에 책 정보 추가
	query := "INSERT INTO bz.dbo.tbl_book (title, author, year, regdate) VALUES (?, ?, ?, GETDATE())"
	_, err = db.Exec(query, book.Title, book.Author, book.Year)
	if err != nil {
		log.Printf("DB 에러: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "책 정보 추가 실패"})
		return
	}

	// 추가된 책 정보 조회
	var newBook Book
	err = db.QueryRow("SELECT TOP 1 id, title, author, year, regdate FROM bz.dbo.tbl_book ORDER BY regdate DESC").
		Scan(&newBook.ID, &newBook.Title, &newBook.Author, &newBook.Year, &newBook.Regdate)
	if err != nil {
		log.Printf("조회 에러: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "추가된 책 정보 조회 실패"})
		return
	}

	books = append(books, newBook)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newBook)
}

// 책 정보 수정
func UpdateBook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := mux.Vars(r)
	id := params["id"]

	var book Book
	err := json.NewDecoder(r.Body).Decode(&book)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "잘못된 요청 형식입니다"})
		return
	}

	// DB에서 책 정보 수정
	query := "UPDATE bz.dbo.tbl_book SET title = ?, author = ?, year = ? WHERE id = ?"
	result, err := db.Exec(query, book.Title, book.Author, book.Year, id)
	if err != nil {
		log.Printf("DB 에러: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "책 정보 수정 실패"})
		return
	}

	// 수정된 행이 있는지 확인
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("행 수 확인 에러: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "수정 결과 확인 실패"})
		return
	}

	if rowsAffected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "수정할 책을 찾을 수 없습니다"})
		return
	}

	// 수정된 책 정보 조회
	var updatedBook Book
	err = db.QueryRow("SELECT id, title, author, year, regdate FROM bz.dbo.tbl_book WHERE id = ?", id).
		Scan(&updatedBook.ID, &updatedBook.Title, &updatedBook.Author, &updatedBook.Year, &updatedBook.Regdate)
	if err != nil {
		log.Printf("조회 에러: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "수정된 책 정보 조회 실패"})
		return
	}

	// 메모리 데이터 업데이트
	for i, b := range books {
		if b.ID == id {
			books[i] = updatedBook
			break
		}
	}

	json.NewEncoder(w).Encode(updatedBook)
}

// 책 삭제
func DeleteBook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := mux.Vars(r)
	id := params["id"]

	// DB에서 책 삭제
	query := "DELETE FROM bz.dbo.tbl_book WHERE id = ?"
	result, err := db.Exec(query, id)
	if err != nil {
		log.Printf("DB 에러: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "책 삭제 실패"})
		return
	}

	// 삭제된 행이 있는지 확인
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("행 수 확인 에러: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "삭제 결과 확인 실패"})
		return
	}

	if rowsAffected == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "삭제할 책을 찾을 수 없습니다"})
		return
	}

	// 메모리에서도 삭제
	for i, book := range books {
		if book.ID == id {
			books = append(books[:i], books[i+1:]...)
			break
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "책이 성공적으로 삭제되었습니다"})
}

func main() {
	// 설정 로드
	config := loadConfig()

	// DB 연결
	connectDB(config)
	defer db.Close()

	// 테이블 구조 확인을 위한 쿼리
	rows, err := db.Query("SELECT TOP 1 * FROM bz.dbo.tbl_book")
	if err != nil {
		log.Fatal("DB 조회 실패:", err)
	}
	defer rows.Close()

	// 컬럼 정보 가져오기
	columns, err := rows.Columns()
	if err != nil {
		log.Fatal("컬럼 정보 조회 실패:", err)
	}
	log.Printf("테이블 컬럼: %v", columns)

	// 실제 데이터 조회
	rows, err = db.Query("SELECT * FROM bz.dbo.tbl_book")
	if err != nil {
		log.Fatal("DB 조회 실패:", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, author string
		var year int
		var regdate time.Time

		err = rows.Scan(&id, &title, &author, &year, &regdate)
		if err != nil {
			log.Fatal("데이터 스캔 실패:", err)
		}

		book := Book{
			ID:      id,
			Title:   title,
			Author:  author,
			Year:    year,
			Regdate: regdate.Format("2006-01-02 15:04:05"),
		}
		books = append(books, book)
	}

	// 인증 미들웨어 생성
	auth := authMiddleware(config.APIKey)

	router := mux.NewRouter()

	// 헬스체크 엔드포인트 (인증 불필요)
	router.HandleFunc("/health", HealthCheck).Methods("GET")

	// API 엔드포인트들
	router.HandleFunc("/books", auth(GetBooks)).Methods("GET")
	router.HandleFunc("/books/{id}", auth(GetBook)).Methods("GET")
	router.HandleFunc("/books", auth(CreateBook)).Methods("POST")
	router.HandleFunc("/books/{id}", auth(UpdateBook)).Methods("PUT")
	router.HandleFunc("/books/{id}", auth(DeleteBook)).Methods("DELETE")

	// 서버 시작
	log.Printf("서버가 포트 %s에서 시작됩니다...", config.Port)
	log.Fatal(http.ListenAndServe(":"+config.Port, router))
}
