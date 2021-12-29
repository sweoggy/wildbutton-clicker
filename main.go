package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/chromedp/chromedp"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
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

	productionMode := os.Getenv("APP_MODE") == "prod"
	if productionMode {
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

		if productionMode {
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
		}

		log.Println("Got slack events body " + string(body))

		eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to parse Slack event: "+err.Error())
			return
		}

		if eventsAPIEvent.Type == slackevents.URLVerification { // If URLVerification, respond with challenge
			var urlVerification SlackUrlVerification

			err = json.Unmarshal(body, &urlVerification)

			if err != nil {
				c.String(http.StatusBadRequest, "Could not unmarshal JSON body "+err.Error())
				return
			}

			c.String(http.StatusOK, urlVerification.Challenge)
		} else if eventsAPIEvent.Type == slackevents.CallbackEvent { // If callback event
			innerEvent := eventsAPIEvent.InnerEvent
			switch ev := innerEvent.Data.(type) {
			case *slackevents.MessageEvent:
				if ev.Text == "A wild BUTTON appears!" {
					pageUrl := "https://app.slack.com/client/" + eventsAPIEvent.TeamID + "/" + ev.Channel
					buttonIdentifier := `[data-qa-action-id="wild_button"]`

					// Connect to Chrome
					ctx, cancel := chromedp.NewRemoteAllocator(context.Background(), getDebugURL())
					defer cancel()

					// ensure that the browser process is started
					ctx, cancel = chromedp.NewContext(ctx)
					defer cancel()

					err := chromedp.Run(ctx,
						chromedp.Navigate(pageUrl),
						// wait for button element to be visible (ie, page is loaded)
						RunWithTimeOut(&ctx, 15, chromedp.Tasks{
							chromedp.WaitVisible(buttonIdentifier),
							// find and click wild button
							chromedp.Click(buttonIdentifier, chromedp.NodeVisible),
						}),
					)

					if err != nil {
						log.Printf("Failed to click button: %v", err)

						// capture screenshot of body
						var buf []byte
						if err := chromedp.Run(ctx, fullScreenshot(pageUrl, 90, &buf)); err != nil {
							log.Printf("Failed to take screenshot: %v", err)
							break
						}
						if err := ioutil.WriteFile("failedbuttonclick.png", buf, 0o644); err != nil {
							log.Printf("Failed to save screenshot: %v", err)
							break
						}

						log.Printf("Screenshot saved successfully")
					} else {
						log.Println("Clicked wild button!")
					}
				}
			}
		} else {
			log.Println("Unknown event " + eventsAPIEvent.Type)
		}
	})

	return r
}

func getDebugURL() string {
	resp, err := http.Get("http://localhost:9222/json/version")
	if err != nil {
		log.Fatal(err)
	}

	var result map[string]interface{}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatal(err)
	}
	return result["webSocketDebuggerUrl"].(string)
}

// fullScreenshot takes a screenshot of the entire browser viewport.
//
// Note: chromedp.FullScreenshot overrides the device's emulation settings. Use
// device.Reset to reset the emulation and viewport settings.
func fullScreenshot(urlstr string, quality int, res *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Navigate(urlstr),
		chromedp.Sleep(5 * time.Second), // TODO: Should be replaced by a WaitVisible
		chromedp.FullScreenshot(res, quality),
	}
}

func RunWithTimeOut(ctx *context.Context, timeout time.Duration, tasks chromedp.Tasks) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		timeoutContext, cancel := context.WithTimeout(ctx, timeout*time.Second)
		defer cancel()
		return tasks.Do(timeoutContext)
	}
}

func main() {
	/* // create chrome instance
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.Flag("headless", false),
	)

	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel() */

	r := setupRouter()
	// Listen and Server in 0.0.0.0:8080
	r.Run(":8080")
}
