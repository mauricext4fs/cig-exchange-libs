package models

import (
    "fmt"
    "os"
    "github.com/jinzhu/gorm"
    _ "github.com/jinzhu/gorm/dialects/postgres"
    "github.com/joho/godotenv"
    "github.com/go-redis/redis"
)

var db *gorm.DB
var redisD *redis.Client

type models struct {
    db *gorm.DB
}


func init() {

    e := godotenv.Load()
    if e != nil {
        fmt.Print(e)
    }

    // PostgreSQL Init
    username := os.Getenv("db_user")
    password := os.Getenv("db_pass")
    dbName := os.Getenv("db_name")
    dbHost := os.Getenv("db_host")
    dbPort := os.Getenv("db_port")

    dbUri := fmt.Sprintf("hossssssssst=%s user=%s dbname=%s sslmode=disable password=%s port=%s", dbHost, username, dbName, password, dbPort)
    fmt.Println(dbUri)

    conn, err := gorm.Open("postgres", dbUri)
    if err != nil {
        fmt.Print(err)
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
    fmt.Println(pong, err)
    redisD = client
}

func GetDB() *gorm.DB {
    return db
}

func GetRedis() *redis.Client {
    return redisD
}
