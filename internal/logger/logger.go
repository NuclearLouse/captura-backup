package logger

import (
	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/orandin/lumberjackrus"
	"github.com/sirupsen/logrus"
)

type Config struct {
	Level         string
	LogFile       string
	FormatTime    string
	MaxSize       int
	MaxBackup     int
	MaxAge        int
	Compress      bool
	Localtime     bool
	ShowFullLevel bool
}

func DefaultConfig() *Config {
	return &Config{
		Level:      "trace",
		MaxSize:    1,
		MaxBackup:  3,
		MaxAge:     1,
		Compress:   true,
		Localtime:  true,
		FormatTime: "2006-01-02 15:04:05.000",
	}
}

func New(cfg *Config) *logrus.Logger {
	log := logrus.New()
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		level = logrus.TraceLevel
	}

	log.SetLevel(level)

	formatter := &nested.Formatter{
		TimestampFormat: cfg.FormatTime,
		ShowFullLevel:   cfg.ShowFullLevel,
		NoColors:        true,
	}

	if cfg.LogFile == "" {
		formatter.NoColors = false
		log.SetFormatter(formatter)
		return log
	}

	log.SetFormatter(formatter)
	hook, _ := lumberjackrus.NewHook(
		&lumberjackrus.LogFile{
			Filename:   cfg.LogFile,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackup,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
			LocalTime:  cfg.Localtime,
		},
		logrus.TraceLevel,
		formatter,
		nil,
	)
	log.AddHook(hook)
	return log
}
