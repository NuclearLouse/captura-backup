package emailer

import (
	"bytes"
	"errors"
	"html/template"
	"net/mail"
	"net/smtp"
	"captura-backup/internal/notification"
	"path/filepath"
	"time"

	"github.com/jordan-wright/email"
)

type Emailer struct {
	*Config
}

type Config struct {
	SmtpUser    string
	SmtpPass    string
	SmtpHost    string
	SmtpPort    string
	VisibleName string
	Timeout     int64
	Addresses   []string
}

func New(cfg *Config) notification.Notificator {
	return &Emailer{cfg}
}

func (e *Emailer) SendMessage(data notification.DataToSend) error {

	if data.Template == "" {
		return errors.New("no template to send")
	}

	body := new(bytes.Buffer)
	err := template.Must(template.ParseFiles(filepath.Join("templates", "email", data.Template+".tmpl"))).Execute(body, data)
	if err != nil {
		return err
	}

	return e.send(body.Bytes(), data.Header, e.Addresses)
}

func (e *Emailer) send(body []byte, header string, addresses []string) error {
	if len(addresses) == 0 {
		return errors.New("no addresses to send")
	}

	from := mail.Address{
		Address: e.SmtpUser,
		Name:    e.VisibleName,
	}

	m := email.NewEmail()
	m.From = from.String()
	m.Subject = header
	m.HTML = body
	m.To = addresses

	pool, err := email.NewPool(
		e.SmtpHost+":"+e.SmtpPort,
		1,
		loginAuth(e.SmtpUser, e.SmtpPass),
	)
	if err != nil {
		return err
	}
	defer pool.Close()

	return pool.Send(m, time.Duration(e.Timeout)*time.Second)
}

type emailUser struct {
	username string
	password string
}

func loginAuth(username, password string) smtp.Auth {
	return &emailUser{username, password}
}

func (a *emailUser) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(a.username), nil
}

func (a *emailUser) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch string(fromServer) {
		case "Username:":
			return []byte(a.username), nil
		case "Password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unknown for server")
		}
	}
	return nil, nil
}
