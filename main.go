package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"database/sql"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

var DB *sql.DB
var jwtSecret = []byte("your_secret_key")

// Struktur Data untuk Makanan
type Food struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	ExpiryDate string `json:"expiry_date"`
}

// Struktur Request untuk Menambahkan Makanan
type AddFoodRequest struct {
	Name       string `json:"name"`
	ExpiryDate string `json:"expiry_date"`
}

// Struktur Request dan Response untuk Resep Makanan
type RecipeRequest struct {
	FoodName string `json:"food_name"`
}

type RecipeResponse struct {
	Recipe string `json:"recipe"`
}

// Struktur Claims untuk JWT
type Claims struct {
	Username string `json:"username"`
	jwt.StandardClaims
}

// Fungsi untuk Inisialisasi Database
func InitDB() {
	var err error
	DB, err = sql.Open("mysql", "root:@tcp(127.0.0.1:3306)/savebite")
	if err != nil {
		log.Fatalf("Gagal terhubung ke database: %v", err)
	}

	err = DB.Ping()
	if err != nil {
		log.Fatalf("Tidak bisa terhubung ke database: %v", err)
	}

	fmt.Println("✅ Berhasil terhubung ke database")
}

// Fungsi untuk Menambah Makanan ke Database
func AddFood(name, expiryDate string) error {
	_, err := DB.Exec("INSERT INTO foods (name, expiry_date) VALUES (?, ?)", name, expiryDate)
	return err
}

// Fungsi untuk Mendapatkan Makanan dari Database
func GetFoods() ([]Food, error) {
	rows, err := DB.Query("SELECT id, name, expiry_date FROM foods")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foods []Food
	for rows.Next() {
		var food Food
		if err := rows.Scan(&food.ID, &food.Name, &food.ExpiryDate); err != nil {
			return nil, err
		}
		foods = append(foods, food)
	}

	return foods, nil
}

// Fungsi untuk Menghapus Makanan Berdasarkan ID
func DeleteFood(id string) error {
	_, err := DB.Exec("DELETE FROM foods WHERE id = ?", id)
	return err
}

// Fungsi untuk Menambah Resep ke Database
func AddFoodRecipe(foodID int, recipe string) error {
	_, err := DB.Exec("INSERT INTO food_recipes (food_id, recipe) VALUES (?, ?)", foodID, recipe)
	return err
}

// Fungsi untuk Menghasilkan JWT Token
func GenerateJWT(username string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		Username: username,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
			Issuer:    "myapp",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// Fungsi untuk Validasi Token JWT
func ValidateToken(c *gin.Context) {
	tokenString := c.GetHeader("Authorization")
	if tokenString == "" {
		c.JSON(401, gin.H{"error": "Token tidak ditemukan"})
		c.Abort()
		return
	}

	tokenString = tokenString[7:]

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("metode signing tidak valid")
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		c.JSON(401, gin.H{"error": "Token tidak valid"})
		c.Abort()
		return
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		c.Set("username", claims.Username)
		c.Next()
	} else {
		c.JSON(401, gin.H{"error": "Token tidak valid"})
		c.Abort()
	}
}

// Fungsi untuk Login dan Menghasilkan Token JWT
func loginHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if req.Username == "user" && req.Password == "userpass" {
		token, err := GenerateJWT(req.Username)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to generate token"})
			return
		}
		c.JSON(200, gin.H{"token": token})
		return
	}

	c.JSON(401, gin.H{"error": "Invalid credentials"})
}

// Fungsi utama untuk menangani routing di Vercel
func handler(w http.ResponseWriter, r *http.Request) {
	router := gin.Default()
	router.GET("/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Hello, World!"})
	})

	// Menjalankan aplikasi Go
	if err := router.Run(":3000"); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}

	// Menangani request
	fmt.Fprintf(w, "Vercel function is running!")
}

// Fungsi utama yang dipanggil Vercel
func HelloHandler(w http.ResponseWriter, r *http.Request) {
	handler(w, r)
}

func main() {
	InitDB()

	r := gin.Default()
	r.POST("/login", loginHandler)
	r.POST("/foods", ValidateToken, func(c *gin.Context) {
		var req AddFoodRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Format request salah"})
			return
		}
		err := AddFood(req.Name, req.ExpiryDate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan makanan"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Makanan berhasil disimpan"})
	})
	r.GET("/foods", ValidateToken, func(c *gin.Context) {
		foods, err := GetFoods()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data"})
			return
		}
		c.JSON(http.StatusOK, foods)
	})
	r.DELETE("/foods/:id", ValidateToken, func(c *gin.Context) {
		id := c.Param("id")
		err := DeleteFood(id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menghapus makanan"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Makanan berhasil dihapus"})
	})
	r.POST("/recipe", ValidateToken, func(c *gin.Context) {
		var req RecipeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Format request salah"})
			return
		}
		apiKey := os.Getenv("API_KEY")
		if apiKey == "" {
			log.Fatal("API key tidak ditemukan! Pastikan sudah diset di environment variable.")
		}
		ctx := context.Background()
		client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
		if err != nil {
			log.Fatalf("Error creating AI client: %v", err)
		}
		defer client.Close()

		userInput := fmt.Sprintf("anggap dirimu adalah chef Berikan resep gampang dan berikan ukuran pasti tapi enak untuk: %s", req.FoodName+"di terakhir tuliskan by Chef SaveBite")

		model := client.GenerativeModel("gemini-1.5-flash")
		resp, err := model.GenerateContent(ctx, genai.Text(userInput))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mendapatkan resep dari AI"})
			return
		}

		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "AI tidak mengembalikan hasil yang valid"})
			return
		}

		var output strings.Builder
		for _, part := range resp.Candidates[0].Content.Parts {
			output.WriteString(fmt.Sprintf("%v\n", part))
		}

		foodID := 1
		err = AddFoodRecipe(foodID, output.String())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan resep ke database"})
			return
		}

		c.JSON(http.StatusOK, RecipeResponse{Recipe: output.String()})
	})

	r.Run(":8080")
}
