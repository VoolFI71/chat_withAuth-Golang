package main

import (
	//"time"
	//"fmt"
	//"encoding/json"
	//"database/sql"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/postgres"
	"github.com/gin-gonic/gin"

	//"github.com/gorilla/websocket"
	"net/http"

	_ "github.com/jackc/pgx/v4/stdlib"

	//"github.com/gin-contrib/sessions/cookie"
	"log"
	//"net/http"
	"chat/internal/db"
	"chat/internal/handlers"
	"chat/internal/middleware"
	"chat/internal/websocket"
	"chat/internal/stream"

	"github.com/golang-jwt/jwt/v4"

	//"os"
	"github.com/joho/godotenv"
)


func main() {
    err := godotenv.Load()
    var jwtSecret = []byte("123")

    if err != nil {
        log.Fatalf("Ошибка загрузки .env файла: %v", err)
    }

    err = db.Connect()
    if err != nil {
        log.Fatalf("Ошибка подключения к базе данных: %v", err)
    }
    defer db.Close() 
    database := db.GetDB()

    router := gin.Default()
    router.Use(cors.New(cors.Config{
        AllowOrigins:     []string{"http://127.0.0.1"}, //адрес фронтенда
        AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}, // Разрешенные методы
        AllowHeaders:     []string{"Authorization", "Content-Type"}, // Разрешенные заголовки
        ExposeHeaders:    []string{"Content-Length"}, // Заголовки, которые могут быть доступны клиенту
        AllowCredentials: true, // Разрешить отправку учетных данных
    }))


    sessionsOptions := sessions.Options{
        MaxAge:   1000,
        HttpOnly: true, 
    }


    store, err := postgres.NewStore(db.GetDB(), []byte("secret"))
    if err != nil {
        log.Fatalf("Ошибка создания хранилища сессий: %v", err)
    }
    
    router.Use(sessions.Sessions("mysession", store))
    router.Use(func(c *gin.Context) {
        session := sessions.Default(c)
        session.Options(sessionsOptions)
        c.Next()
    })


    go websocket.HandleMessages()


    router.GET("/gt", middleware.AuthMiddleware(), handlers.GT)
    router.GET(`/`, handlers.MainPage)
    router.GET("/wsstream", stream.Stream)
    router.GET("/ws", websocket.SendMsg(database))

    router.GET("/getmsg", websocket.GetMessagesHandler(database))
    router.POST("/savemsg",  middleware.AuthMiddleware(), websocket.SaveMsg(database))

    router.POST("/sendmail", handlers.Sendmail(database))
    router.POST("/login", handlers.Login(database))
    router.POST("/reg", handlers.Reg(database))
    router.GET("/userinfo", func(c *gin.Context) {
        tokenString := c.GetHeader("Authorization")
        
        // Удаляем "Bearer " из токена, если он присутствует
        if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
            tokenString = tokenString[7:]
        }

        if tokenString == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization token is required"})
            return
        }

        token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, http.ErrNotSupported
            }
            return jwtSecret, nil
        })

        if err != nil || !token.Valid {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
            return
        }

        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok || !token.Valid {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
            return
        }

        username, ok := claims["username"].(string)
        if !ok {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Username not found in token"})
            return
        }

        c.JSON(http.StatusOK, gin.H{
            "username": username,
        })
    })


    if err := router.Run(":8080"); err != nil {
        panic(err)
    }
} 

