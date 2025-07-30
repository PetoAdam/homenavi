package db

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func MustInitDB() {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("POSTGRES_HOST"),
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		os.Getenv("POSTGRES_DB"),
		os.Getenv("POSTGRES_PORT"),
	)
	var err error
	for i := 0; i < 30; i++ {
		DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			if err = DB.AutoMigrate(&User{}, &EmailVerification{}); err == nil {
				log.Println("Connected to Postgres and migrated schema")
				// Create default admin user if not exists
				adminEmail := "admin@example.com"
				adminUser := "admin"
				var existing User
				resp := DB.Where("email = ?", adminEmail).Or("user_name = ?", adminUser).First(&existing)
				if resp.Error == nil {
					log.Printf("[INFO] Default admin user already exists: %s", existing.Email)
				} else {
					hash, _ := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
					ph := string(hash)
					user := User{
						ID:                 uuid.New(),
						UserName:           adminUser,
						NormalizedUserName: strings.ToUpper(adminUser),
						Email:              adminEmail,
						NormalizedEmail:    strings.ToUpper(adminEmail),
						FirstName:          "",
						LastName:           "",
						Role:               "admin",
						EmailConfirmed:     false,
						PasswordHash:       &ph,
						TwoFactorEnabled:   false,
						LockoutEnabled:     false,
						AccessFailedCount:  0,
					}
					if err := DB.Create(&user).Error; err != nil {
						log.Printf("[ERROR] Failed to create default admin user: %v", err)
					} else {
						log.Printf("[INFO] Default admin user created: %s", adminEmail)
					}
				}
				return
			}
		}
		log.Printf("Waiting for Postgres to be ready (%d/30)...", i+1)
		if i == 29 {
			log.Fatalf("Failed to connect to Postgres after retries: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
}

type User struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserName           string     `gorm:"uniqueIndex" json:"user_name"`
	NormalizedUserName string     `json:"normalized_user_name"`
	Email              string     `gorm:"uniqueIndex" json:"email"`
	NormalizedEmail    string     `json:"normalized_email"`
	FirstName          string     `json:"first_name"`
	LastName           string     `json:"last_name"`
	Role               string     `json:"role" gorm:"type:varchar(16);default:'user'"`
	EmailConfirmed     bool       `json:"email_confirmed"`
	ProfilePictureURL  *string    `json:"profile_picture_url" gorm:"type:varchar(255)"`
	PasswordHash       *string    `json:"password_hash"`
	GoogleID           *string    `gorm:"uniqueIndex" json:"google_id"`
	TwoFactorEnabled   bool       `json:"two_factor_enabled"`
	TwoFactorType      string     `json:"two_factor_type" gorm:"type:varchar(16)"`
	TwoFactorSecret    string     `json:"two_factor_secret" gorm:"type:varchar(64)"` // TOTP secret for 2FA
	LockoutEnd         *time.Time `json:"lockout_end"`
	LockoutEnabled     bool       `json:"lockout_enabled"`
	AccessFailedCount  int        `json:"access_failed_count"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type EmailVerification struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;index"`
	Code      string    `gorm:"size:16;index"`
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}
