package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/slack-go/slack"
)

type SlackUrlVerification struct {
	Token     string `json:"token"`
	Challenge string `json:"challenge"`
	Type      string `json:"type"`
}

func setupRouter() *gin.Engine {
	// load .env file
	err := godotenv.Load()

	if err != nil {
		log.Fatalf("Error loading .env file")
	}

	if os.Getenv("APP_MODE") == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Disable Console Color
	// gin.DisableConsoleColor()
	r := gin.Default()

	// Ping test
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	r.POST("/slack-events", func(c *gin.Context) {
		body, err := ioutil.ReadAll(c.Request.Body)

		if err != nil {
			c.String(http.StatusBadRequest, "Could not read body")
			return
		}

		slackSigningSecret := os.Getenv("SLACK_SIGNING_SECRET")
		sv, err := slack.NewSecretsVerifier(c.Request.Header, slackSigningSecret)

		if err != nil {
			c.String(http.StatusBadRequest, "Bad/missing signing secret")
			return
		}
		if _, err := sv.Write(body); err != nil {
			c.String(http.StatusInternalServerError, "Invalid body for signing secret: "+err.Error())
			return
		}
		if err := sv.Ensure(); err != nil {
			c.String(http.StatusUnauthorized, "Invalid signing secret: "+err.Error())
			return
		}

		log.Println("Got slack events body " + string(body))

		// Parse JSON
		var urlVerification SlackUrlVerification

		err = json.Unmarshal(body, &urlVerification)

		if err != nil {
			c.String(http.StatusBadRequest, "Could not unmarshal JSON body "+err.Error())
			return
		}

		c.String(http.StatusOK, urlVerification.Challenge)
	})

	return r
}

func main() {
	r := setupRouter()
	// Listen and Server in 0.0.0.0:8080
	r.Run(":8080")
}
