package bitrixer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"text/template"
	"time"

	"captura-backup/internal/notification"
	"captura-backup/internal/request"
)

const (
	requestMessage  = "im.message.add.json"
	requestDelete   = "im.message.delete"
	requestNotify   = "im.notify.system.add.json"
	reqParamMessId  = "MESSAGE_ID"
	reqParamDialog  = "DIALOG_ID"
	reqParamUserId  = "USER_ID"
	reqParamMessage = "MESSAGE"
	reqParamSystem  = "SYSTEM"
	// reqParamAttach  = "ATTACH"
)

type Bitrixer struct {
	*Config
}

type Config struct {
	RestUrl         string
	Token           string
	UserId          string
	Timeout         int64
	LifetimeMessage int64
	Addresses       []string
	GlobalChat      string
	GlobalChatUsers []string
}

func New(cfg *Config) notification.Notificator {
	return &Bitrixer{cfg}
}

func (b *Bitrixer) requestPath(request string) string {
	return fmt.Sprintf("/rest/%s/%s/%s", b.UserId, b.Token, request)
}

func (b *Bitrixer) urlForMessage(dialogId, message string) string {
	return request.MethodApiURL(&request.MethodApi{
		Host: b.RestUrl,
		Path: b.requestPath(requestMessage),
		Params: map[string]string{
			reqParamSystem:  "Y",
			reqParamDialog:  dialogId,
			reqParamMessage: message,
		},
	})
}

func (b *Bitrixer) urlForDelete(messageId string) string {
	return request.MethodApiURL(&request.MethodApi{
		Host: b.RestUrl,
		Path: b.requestPath(requestDelete),
		Params: map[string]string{
			reqParamMessId: messageId,
		},
	})
}

func (b *Bitrixer) urlForNotify(userId, message string) string {
	return request.MethodApiURL(&request.MethodApi{
		Host: b.RestUrl,
		Path: b.requestPath(requestNotify),
		Params: map[string]string{
			reqParamUserId:  userId,
			reqParamMessage: message,
		},
	})
}

func (b *Bitrixer) SendMessage(data notification.DataToSend) error {

	if len(b.Addresses) == 0 {
		return errors.New("no addresses to send")
	}

	users := b.Addresses
	addresses := b.Addresses
	if data.UseGlobalNotification {
		addresses = []string{b.GlobalChat}
		users = b.GlobalChatUsers
	}

	for _, user := range users {
		url := b.urlForNotify(user, data.Header)
		if _, err := b.send(url); err != nil {
			return err
		}
	}

	body := new(bytes.Buffer)
	err := template.Must(template.ParseFiles(filepath.Join("templates", "bitrix", data.Template+".tmpl"))).Execute(body, data)
	if err != nil {
		return err
	}

	for _, chat := range addresses {
		url := b.urlForMessage(chat, body.String())
		mesId, err := b.send(url)
		if err != nil {
			return err
		}
		go func() {
			time.Sleep(time.Duration(b.LifetimeMessage) * time.Hour)
			b.send(b.urlForDelete(fmt.Sprintf("%d", mesId)))
		}()
	}

	return nil
}

func (b *Bitrixer) send(url string) (int64, error) {

	res, err := request.NewRequest(&request.Params{
		URL:     url,
		Timeout: b.Timeout,
	})
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	var result struct {
		Result int64 `json:"result"`
		Time   struct {
			Start      float64   `json:"start"`
			Finish     float64   `json:"finish"`
			Duration   float64   `json:"duration"`
			Processing float64   `json:"processing"`
			DateStart  time.Time `json:"date_start"`
			DateFinish time.Time `json:"date_finish"`
		} `json:"time"`
	}

	if res.StatusCode == 200 {
		//Success:
		// при удалении сообщения "result": 868197, type int64
		// при удалении сообщения "result": true, type bool - не обрабатываю результат
		if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
			return result.Result, fmt.Errorf("decode result json: %w", err)
		}

		if result.Result == 0 {
			return result.Result, errors.New("no result in the response")
		}

		return result.Result, nil
	}
	//Error:
	var resultErr struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resultErr); err != nil {
		return result.Result, fmt.Errorf("decode result json: %w", err)
	}
	if resultErr.Error != "" {
		return result.Result, fmt.Errorf("%s: %s", resultErr.Error, resultErr.ErrorDescription)
	}
	return 0, errors.New("unsupported bitrix-api response")
}
