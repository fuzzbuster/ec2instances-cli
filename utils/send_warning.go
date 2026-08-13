package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/imroc/req/v3"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

var notificationHTTPClient = req.C().SetTimeout(5 * time.Second).DisableAutoDecode()

type warningNotifier struct {
	name string
	send func(string) error
}

func configuredWarningNotifiers() []warningNotifier {
	notifiers := []warningNotifier{
		{
			name: "Slack",
			send: sendSlackNotification,
		},
		{
			name: "Feishu",
			send: sendFeishuNotification,
		},
		{
			name: "ntfy",
			send: sendNtfyNotification,
		},
		{
			name: "Bark",
			send: sendBarkNotification,
		},
	}

	configured := notifiers[:0]
	for _, notifier := range notifiers {
		if notifier.configured() {
			configured = append(configured, notifier)
		}
	}
	return configured
}

func (notifier warningNotifier) configured() bool {
	switch notifier.name {
	case "Slack":
		return os.Getenv("SLACK_WEBHOOK_URL") != ""
	case "Feishu":
		return os.Getenv("FEISHU_WEBHOOK_URL") != ""
	case "ntfy":
		return os.Getenv("NTFY_URL") != ""
	case "Bark":
		return os.Getenv("BARK_URL") != "" && os.Getenv("BARK_DEVICE_KEY") != ""
	default:
		return false
	}
}

func sendSlackNotification(message string) error {
	response, err := notificationHTTPClient.R().
		SetBodyJsonMarshal(map[string]string{
			"text": message,
		}).
		Post(os.Getenv("SLACK_WEBHOOK_URL"))
	if err != nil {
		return err
	}
	return validateNotificationHTTPStatus(response)
}

func sendFeishuNotification(message string) error {
	payload := map[string]any{
		"msg_type": "text",
		"content": map[string]string{
			"text": message,
		},
	}

	if secret := os.Getenv("FEISHU_SECRET"); secret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		payload["timestamp"] = timestamp
		payload["sign"] = feishuSignature(secret, timestamp)
	}

	response, err := notificationHTTPClient.R().
		SetBodyJsonMarshal(payload).
		Post(os.Getenv("FEISHU_WEBHOOK_URL"))
	if err != nil {
		return err
	}
	if err := validateNotificationHTTPStatus(response); err != nil {
		return err
	}

	var result feishuResponse
	if err := json.Unmarshal(response.Bytes(), &result); err != nil {
		return err
	}
	if result.Code == nil {
		return fmt.Errorf("missing business code: %s", response.String())
	}
	if *result.Code != 0 {
		return fmt.Errorf("business code %d: %s", *result.Code, result.Msg)
	}
	return nil
}

func feishuSignature(secret, timestamp string) string {
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

type feishuResponse struct {
	Code *int   `json:"code"`
	Msg  string `json:"msg"`
}

func sendNtfyNotification(message string) error {
	request := notificationHTTPClient.R().
		SetHeader("Content-Type", "text/plain; charset=utf-8").
		SetBodyString(message)

	if authHeader := os.Getenv("NTFY_AUTH_HEADER"); authHeader != "" {
		request.SetHeader("Authorization", authHeader)
	} else if token := os.Getenv("NTFY_TOKEN"); token != "" {
		request.SetBearerAuthToken(token)
	}

	response, err := request.Post(os.Getenv("NTFY_URL"))
	if err != nil {
		return err
	}
	return validateNotificationHTTPStatus(response)
}

func sendBarkNotification(message string) error {
	response, err := notificationHTTPClient.R().
		SetBodyJsonMarshal(map[string]string{
			"device_key": os.Getenv("BARK_DEVICE_KEY"),
			"title":      "WARNING",
			"body":       strings.TrimSpace(message),
		}).
		Post(strings.TrimRight(os.Getenv("BARK_URL"), "/") + "/push")
	if err != nil {
		return err
	}
	if err := validateNotificationHTTPStatus(response); err != nil {
		return err
	}

	var result barkResponse
	if err := json.Unmarshal(response.Bytes(), &result); err != nil {
		return err
	}
	if result.Code == nil {
		return fmt.Errorf("missing business code: %s", response.String())
	}
	if *result.Code != 0 && *result.Code != 200 {
		return fmt.Errorf("business code %d: %s", *result.Code, result.Message)
	}
	return nil
}

type barkResponse struct {
	Code    *int   `json:"code"`
	Message string `json:"message"`
}

func validateNotificationHTTPStatus(response *req.Response) error {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s", response.Status)
	}
	return nil
}

func sendWarningNotification(notifier warningNotifier, message string) {
	if err := notifier.send(message); err != nil {
		log.Default().Println("Error sending", notifier.name, "notification:", err)
	}
}

// SendWarning sends a warning to the console and configured notification channels.
func SendWarning(warningData ...any) {
	warningData = append([]any{"WARNING:"}, warningData...)
	x := fmt.Sprintln(warningData...)
	log.Default().Println(x)

	for _, notifier := range configuredWarningNotifiers() {
		sendWarningNotification(notifier, x)
	}
}
