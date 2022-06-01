package service

import (
	"captura-backup/internal/notification"
	"captura-backup/internal/notification/bitrixer"
	"captura-backup/internal/notification/emailer"
	"captura-backup/internal/notification/telegramer"
	"captura-backup/internal/storage"
	"captura-backup/internal/storage/local"
	"captura-backup/internal/storage/remote/ftp"
	"captura-backup/internal/storage/remote/sftp"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v4/pgxpool"
)

func (s *Service) connectDatabase(ctx context.Context) (*pgxpool.Pool, error) {
	return pgxpool.Connect(ctx,
		fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s&pool_max_conns=%d",
			s.ini.Section("database").Key("user").MustString("postgres"),
			s.ini.Section("database").Key("password").MustString("postgres"),
			s.ini.Section("database").Key("host").MustString("localhost"),
			s.ini.Section("database").Key("port").MustString("5432"),
			s.ini.Section("database").Key("database").String(),
			s.ini.Section("database").Key("sslmode").MustString("disable"),
			int32(s.ini.Section("database").Key("max_open_conns").MustInt(25)),
		))
}

func (s *Service) filesProducer() (storage.Producer, error) {
	cfg := &storage.RemoteConfig{
		Host:           s.ini.Section("remote.cfg").Key("host").String(),
		Port:           s.ini.Section("remote.cfg").Key("port").String(),
		AuthMethod:     s.ini.Section("remote.cfg").Key("auth_method").String(),
		User:           s.ini.Section("remote.cfg").Key("user").String(),
		Password:       s.ini.Section("remote.cfg").Key("pass").String(),
		PrivateKeyFile: s.ini.Section("remote.cfg").Key("privat_key").String(),
		Timeout:        s.ini.Section("remote.cfg").Key("timeout").MustInt64(10),
	}

	network := s.ini.Section("storage").Key("use_remote").MustString("local")

	var producer storage.Producer

	switch network {
	case "local":
		return local.NewProducer(), nil
	case "sftp":
		client, err := sftp.NewClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("new sftp client: %w", err)
		}
		producer = sftp.NewProducer(client)

	case "ftp":
		client, err := ftp.NewClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("new ftp client: %w", err)
		}
		producer = ftp.NewProducer(client)

	default:
		return nil, errors.New("[" + network + "] unsupported protocol")
	}

	if err := producer.Ping(); err != nil {
		return nil, fmt.Errorf("remote %s failed ping : %w", network, err)
	}

	return producer, nil
}

func (s *Service) notificators() {
	var senders []notification.Notificator

	messengers := s.ini.Section("notification").Key("enabled").Strings(",")

	for _, messenger := range messengers {
		var (
			sender notification.Notificator
		)
		switch messenger {
		case "email":
			cfg := &emailer.Config{
				SmtpUser:    s.ini.Section(messenger + ".cfg").Key("smtp_user").String(),
				SmtpPass:    s.ini.Section(messenger + ".cfg").Key("smtp_pass").String(),
				SmtpHost:    s.ini.Section(messenger + ".cfg").Key("smtp_host").String(),
				SmtpPort:    s.ini.Section(messenger + ".cfg").Key("smtp_port").String(),
				VisibleName: s.ini.Section(messenger + ".cfg").Key("visible_name").String(),
				Addresses:   s.ini.Section(messenger + ".cfg").Key("addresses").Strings(","),
				Timeout:     s.ini.Section(messenger + ".cfg").Key("timeout").MustInt64(5),
			}
			sender = emailer.New(cfg)
		case "telegram":
			cfg := &telegramer.Config{
				RestUrl:   s.ini.Section(messenger + ".cfg").Key("rest_url").String(),
				Token:     s.ini.Section(messenger + ".cfg").Key("token").String(),
				Addresses: s.ini.Section(messenger + ".cfg").Key("addresses").Ints(","),
				Timeout:   s.ini.Section(messenger + ".cfg").Key("timeout").MustInt64(5),
			}
			sender = telegramer.New(cfg)
		case "bitrix":
			cfg := &bitrixer.Config{
				RestUrl:         s.ini.Section(messenger + ".cfg").Key("rest_url").String(),
				Token:           s.ini.Section(messenger + ".cfg").Key("token").String(),
				UserId:          s.ini.Section(messenger + ".cfg").Key("user_id").String(),
				Addresses:       s.ini.Section(messenger + ".cfg").Key("addresses").Strings(","),
				Timeout:         s.ini.Section(messenger + ".cfg").Key("timeout").MustInt64(5),
				GlobalChat:      s.ini.Section(messenger + ".cfg").Key("global_chat").String(),
				GlobalChatUsers: s.ini.Section(messenger + ".cfg").Key("global_chat_users").Strings(","),
				LifetimeMessage: s.ini.Section(messenger + ".cfg").Key("lifetime_message").MustInt64(72),
			}
			sender = bitrixer.New(cfg)
		}

		if sender != nil {
			senders = append(senders, sender)
		}

	}

	s.senders = senders
}
