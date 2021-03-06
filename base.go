package cigExchange

import (
	"cig-exchange-libs/twilio"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/duo-labs/webauthn/webauthn"
	"github.com/go-redis/redis"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres" // postgresql driver
	"github.com/joho/godotenv"
	"github.com/mattbaird/gochimp"
)

var (
	db             *gorm.DB
	redisD         *redis.Client
	twilioOTP      *twilio.OTP
	web            *webauthn.WebAuthn
	mandrillClient *gochimp.MandrillAPI
)
var isDevEnvironment bool

func init() {

	// Random init
	rand.Seed(time.Now().UnixNano())

	err := godotenv.Load()
	if err != nil {
		fmt.Print(err)
	}

	// Determine environment type
	if os.Getenv("ENV") == "dev" {
		isDevEnvironment = true
	}

	// Twilio Init
	twilioAPIKey := os.Getenv("TWILIO_APIKEY")
	twilioOTP = twilio.NewOTP(twilioAPIKey)

	// Mandrill Init
	mandrillKey := os.Getenv("MANDRILL_KEY")
	mandrillClient, err = gochimp.NewMandrill(mandrillKey)
	if err != nil {
		fmt.Print(err)
	}

	// WebAuthn init
	displayName := "cig-exchange.ch"
	rpID := "cig-exchange.ch"
	// development settings
	if IsDevEnv() {
		displayName = "localhost"
		rpID = "localhost"
	}
	web, err = webauthn.New(&webauthn.Config{
		RPDisplayName: displayName, // Display Name for your site
		RPID:          rpID,        // Generally the FQDN for your site
	})
	if err != nil {
		fmt.Println(err)
	}

	// PostgreSQL Init
	username := os.Getenv("DB_USER")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")

	dbURI := fmt.Sprintf("host=%s user=%s dbname=%s sslmode=require port=%s", dbHost, username, dbName, dbPort)
	fmt.Println(dbURI)

	conn, err := gorm.Open("postgres", dbURI)
	if err != nil {
		fmt.Println(err)
		reconnectTimeoutSeconds := 15
		fmt.Printf("Database container can be still starting... reconnecting in %d seconds\n", reconnectTimeoutSeconds)
		time.Sleep(time.Second * time.Duration(reconnectTimeoutSeconds))
		conn, err = gorm.Open("postgres", dbURI)
		if err != nil {
			fmt.Printf("Failed to reconnect: %v\n", err.Error())
		}
	}

	db = conn

	// Redis Init

	redisHost := os.Getenv("REDIS_HOST")
	redisPort := os.Getenv("REDIS_PORT")
	client := redis.NewClient(&redis.Options{
		Addr:     redisHost + ":" + redisPort,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	fmt.Println("connecting to Redis...")
	pong, err := client.Ping().Result()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Println(pong)
	redisD = client
}

// GetDB returns a gorm database object singletone
func GetDB() *gorm.DB {
	return db
}

// GetRedis returns a redis client object singletone
func GetRedis() *redis.Client {
	return redisD
}

// GetTwilio returns a wilio OTP object singletone
func GetTwilio() *twilio.OTP {
	return twilioOTP
}

// GetMandrill returns a mandrill object singletone
func GetMandrill() *gochimp.MandrillAPI {
	return mandrillClient
}

// GetWebAuthn returns a web authn object singletone
func GetWebAuthn() *webauthn.WebAuthn {
	return web
}

// IsDevEnv returns true for development environment
func IsDevEnv() bool {
	return isDevEnvironment
}

// GetServerURL return Dev or Prod urls.
// TODO: Need to add staging and prod local urls
func GetServerURL() string {

	if IsDevEnv() {
		return "http://dev.cig-exchange.ch:8228"
	}
	return "https://www.cig-exchange.ch"
}
