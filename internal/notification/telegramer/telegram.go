package telegramer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"captura-backup/internal/notification"
	"captura-backup/internal/request"
	"path/filepath"
	"text/template"
)

const (
	requestMessage = "sendMessage"
)

type Telegramer struct {
	*Config
}

type Config struct {
	RestUrl   string
	Token     string
	Timeout   int64
	Addresses []int
}

func New(cfg *Config) notification.Notificator {
	return &Telegramer{cfg}
}

func (t *Telegramer) requestPath(request string) string {
	return fmt.Sprintf("/bot%s/%s", t.Token, request)
}

func (t *Telegramer) SendMessage(data notification.DataToSend) error {

	body := new(bytes.Buffer)
	err := template.Must(template.ParseFiles(filepath.Join("templates", "telegram", data.Template+".tmpl"))).Execute(body, data)
	if err != nil {
		return err
	}

	for _, chat := range t.Addresses {
		if err := t.send(chat, body.String()); err != nil {
			return err
		}
	}
	return nil
}

func (t *Telegramer) send(chatId int, text string) error {
	reqBody := struct {
		ChatId    int    `json:"chat_id"`
		Text      string `json:"text"`
		ParseMode string `json:"parse_mode"`
	}{
		ChatId:    chatId,
		Text:      text,
		ParseMode: "html",
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal body JSON: %w", err)
	}

	res, err := request.NewRequest(&request.Params{
		URL: request.MethodApiURL(&request.MethodApi{
			Host: t.RestUrl,
			Path: t.requestPath(requestMessage),
		}),
		Timeout: t.Timeout,
		Body:    bodyJSON,
		Header: map[string]string{
			"Content-Type": "application/json",
		},
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		var response struct {
			OK          bool   `json:"ok"`
			ErrorCode   int    `json:"error_code"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
			return fmt.Errorf("decode json errResponse: %w", err)
		}
		if response.OK {
			return fmt.Errorf("error:%d: %s", response.ErrorCode, response.Description)
		}
		return errors.New("unsupported telegram-api response")
	}
	return nil
}
