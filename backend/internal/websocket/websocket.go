package websocket

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/gorilla/websocket"
	"encoding/base64"

)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type ChatMessage struct {
	Username  string `json:"username"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"` 
	Image     string `json:"image"`
}

var clients = make(map[*websocket.Conn]bool)
var broadcast = make(chan ChatMessage)

func SendMsg(db *sql.DB)  gin.HandlerFunc { // функция для вебсокета
	return func (c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			fmt.Println("Error while upgrading connection:", err)
			return
		}
		defer conn.Close()

		clients[conn] = true

		for {
			var msg ChatMessage
			err := conn.ReadJSON(&msg)
			if err != nil {
				fmt.Println("Error while reading message:", err)
				delete(clients, conn)
				break
			}
			
			go func(message ChatMessage) {
				broadcast <- message
			}(msg)
		}
	}
}


func SaveMsg(db *sql.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        var jwtSecret = []byte("123")

        tokenString := c.GetHeader("Authorization")
        if len(tokenString) > 7 && strings.ToLower(tokenString[:7]) == "bearer " {
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

        // Извлечение логина пользователя из токена
        claims, ok := token.Claims.(jwt.MapClaims)
        if !ok || !token.Valid {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
            return
        }
		username, ok := claims["username"].(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Username not found in token claims"})
			return
		}

		message := c.PostForm("message")

        image, err := c.FormFile("image")
        var imageData []byte
        if err == nil {
            file, err := image.Open()
            if err != nil {
                c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open image"})
                return
            }
            defer file.Close()

            imageData, err = io.ReadAll(file)
            if err != nil {
                c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read image"})
                return
            }
        }

        go func() {
			_, err = db.Exec("INSERT INTO chat (chat_id, username, message, image) VALUES ($1, $2, $3, $4)", 1, username, message, imageData)
			if err != nil {
				fmt.Println("Failed to save message:", err)
				return
			}
		}()

        c.JSON(http.StatusOK, gin.H{"status": "Message saved", "username":  username})
    }
}

func GetMessagesHandler(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		messages, err := GetLastMessages(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to fetch messages"})
			return
		}
		c.JSON(http.StatusOK, messages)
	}
}

func GetLastMessages(db *sql.DB) ([]ChatMessage, error) {
	rows, err := db.Query("SELECT username, message, created_at, image FROM chat WHERE chat_id=1 ORDER BY created_at DESC LIMIT 75")
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
        var msg ChatMessage
        var imageData []byte
        if err := rows.Scan(&msg.Username, &msg.Message, &msg.CreatedAt, &imageData); err != nil {
            return nil, err
        }
        if imageData != nil {
            msg.Image = base64.StdEncoding.EncodeToString(imageData)
        }
        messages = append(messages, msg)
    }

	if err := rows.Err(); err != nil {
		fmt.Println(err)	
		return nil, err
	}
	return messages, nil
}




func HandleMessages() {
	for {
		msg := <-broadcast
		go func(message ChatMessage) {
			for client := range clients {
				err := client.WriteJSON(message)
				if err != nil {
					fmt.Println("Error while writing message:", err)
					client.Close()
					delete(clients, client)
				}
			}
		}(msg)
	}
}