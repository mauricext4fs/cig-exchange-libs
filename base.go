package cigExchange

import (
	"cig-exchange-libs/twilio"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/go-redis/redis"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/joho/godotenv"
	"github.com/mattbaird/gochimp"
)

var db *gorm.DB
var redisD *redis.Client
var twilioOTP *twilio.OTP
var mandrillClient *gochimp.MandrillAPI

func init() {

	// Random init
	rand.Seed(time.Now().UnixNano())

	err := godotenv.Load()
	if err != nil {
		fmt.Print(err)
	}

	// Twilio Init
	twilioAPIKey := os.Getenv("twilio_apikey")
	twilioOTP = twilio.NewOTP(twilioAPIKey)

	// Mandrill Init
	mandrillKey := os.Getenv("MANDRILL_KEY")
	mandrillClient, err = gochimp.NewMandrill(mandrillKey)
	if err != nil {
		fmt.Print(err)
	}

	// PostgreSQL Init
	username := os.Getenv("db_user")
	password := os.Getenv("db_pass")
	dbName := os.Getenv("db_name")
	dbHost := os.Getenv("db_host")
	dbPort := os.Getenv("db_port")

	dbURI := fmt.Sprintf("host=%s user=%s dbname=%s sslmode=require password=%s port=%s", dbHost, username, dbName, password, dbPort)
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
	//db.Debug().AutoMigrate(&Account{}, &Contact{})

	// Redis Init
	client := redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
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
